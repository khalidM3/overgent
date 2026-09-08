//go:build windows

package credential

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows credential storage is the Credential Manager, reached through
// CredWriteW / CredReadW / CredDeleteW in advapi32. golang.org/x/sys/windows
// v0.47.0 exposes no wrappers for the Cred* family (it has the service,
// registry and security APIs but not this one), so the three calls are
// declared here over windows.NewLazySystemDLL. Nothing else in the package
// touches the syscall layer.
//
// Credentials are CRED_TYPE_GENERIC with CRED_PERSIST_LOCAL_MACHINE: they
// belong to this user on this machine and are not pushed into a roaming
// profile, which matches a device credential that is meaningless on another
// machine.

const (
	credTypeGeneric         uint32 = 1 // CRED_TYPE_GENERIC
	credPersistLocalMachine uint32 = 2 // CRED_PERSIST_LOCAL_MACHINE

	// credMaxCredentialBlobSize is CRED_MAX_CREDENTIAL_BLOB_SIZE (5*512).
	credMaxCredentialBlobSize = 5 * 512
)

const (
	// errorNotFound is ERROR_NOT_FOUND: no credential with that target name.
	errorNotFound syscall.Errno = 1168
	// errorNoSuchLogonSession is ERROR_NO_SUCH_LOGON_SESSION, which is what a
	// process with no interactive credential set gets back (some service
	// accounts, and a few container configurations).
	errorNoSuchLogonSession syscall.Errno = 1312
)

// credentialW mirrors the CREDENTIALW structure. Field order and types match
// the C declaration exactly; Go's natural alignment reproduces the C layout on
// every Windows architecture Overgent targets.
type credentialW struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         *byte
	TargetAlias        *uint16
	UserName           *uint16
}

var (
	advapi32        = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

func unavailable(reason, remedy string, cause error) error {
	return &UnavailableError{Platform: "windows", Reason: reason, Remedy: remedy, Cause: cause}
}

// findProcs resolves the Cred* entry points. A Windows build without them is
// not a machine Overgent can store a secret on, and that is an unavailable
// store, not a transient failure.
func findProcs(procs ...*windows.LazyProc) error {
	for _, proc := range procs {
		if err := proc.Find(); err != nil {
			return unavailable(
				"the Windows Credential Manager API is not available on this system",
				"run Overgent on a Windows edition that provides advapi32 Cred* (Windows 10 or later, or Server 2016 or later)",
				err,
			)
		}
	}
	return nil
}

// callErrno turns a BOOL-returning Cred* result into an error. LazyProc.Call
// always returns a non-nil error holding the last-error value, which is
// meaningless when the call succeeded, so only the failure path reads it.
func callErrno(succeeded uintptr, lastErr error, operation string) error {
	if succeeded != 0 {
		return nil
	}
	errno, ok := lastErr.(syscall.Errno)
	if !ok || errno == 0 {
		return fmt.Errorf("%s: the Windows Credential Manager reported failure without an error code", operation)
	}
	switch errno {
	case errorNotFound:
		return fmt.Errorf("%s: %w", operation, ErrNotFound)
	case errorNoSuchLogonSession:
		return unavailable(
			"this process has no logon session credential set, so the Credential Manager is unreachable",
			"run Overgent as an interactive user rather than under a service account without a loaded profile",
			errno,
		)
	default:
		return fmt.Errorf("%s: %w", operation, errno)
	}
}

// targetPointer converts an account into a validated UTF-16 TargetName.
func targetPointer(account string) (*uint16, error) {
	target := windowsTargetName(account)
	if len(target) > windowsMaxTargetNameLength {
		return nil, fmt.Errorf("credential account is too long for the Windows Credential Manager (%d of %d bytes)", len(target), windowsMaxTargetNameLength)
	}
	// The account has already been checked for CR and LF; UTF16PtrFromString
	// additionally rejects an embedded NUL, which would truncate the target.
	pointer, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return nil, fmt.Errorf("build the Windows Credential Manager target name: %w", err)
	}
	return pointer, nil
}

func put(_ context.Context, account, secret string) error {
	if err := validatePut(account, secret); err != nil {
		return err
	}
	if err := findProcs(procCredWriteW); err != nil {
		return err
	}
	target, err := targetPointer(account)
	if err != nil {
		return err
	}
	// The blob is opaque to Windows; only this package reads it back, so it is
	// stored as UTF-8 with no terminator, matching what get decodes.
	blob := []byte(secret)
	defer zeroBytes(blob)
	if len(blob) > credMaxCredentialBlobSize {
		return fmt.Errorf("credential secret is too long for the Windows Credential Manager (%d of %d bytes)", len(blob), credMaxCredentialBlobSize)
	}
	user, err := windows.UTF16PtrFromString(account)
	if err != nil {
		return fmt.Errorf("build the Windows Credential Manager user name: %w", err)
	}
	stored := credentialW{
		Type:               credTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
		UserName:           user,
	}
	// CredWriteW overwrites an existing target, which is the upsert the macOS
	// -U flag and the Secret Service replace flag also give us.
	result, _, lastErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&stored)), 0)
	// LazyProc.Call is variadic, so the compiler's unsafe.Pointer-to-uintptr
	// syscall rule does not cover this call site. Keep the struct, and through
	// it the blob and both UTF-16 buffers, reachable until the call returns.
	runtime.KeepAlive(&stored)
	return callErrno(result, lastErr, "store the credential in the Windows Credential Manager")
}

func get(_ context.Context, account string) (string, error) {
	if err := validateAccount(account); err != nil {
		return "", err
	}
	if err := findProcs(procCredReadW, procCredFree); err != nil {
		return "", err
	}
	target, err := targetPointer(account)
	if err != nil {
		return "", err
	}
	var stored *credentialW
	result, _, lastErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(target)),
		uintptr(credTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&stored)),
	)
	runtime.KeepAlive(target)
	if err := callErrno(result, lastErr, "read the credential from the Windows Credential Manager"); err != nil {
		return "", err
	}
	if stored == nil {
		return "", errors.New("the Windows Credential Manager returned no credential")
	}
	// Copy out, wipe the OS-allocated blob, then free it. The buffer is ours
	// to write until CredFree, so the secret does not sit in freed heap.
	size := int(stored.CredentialBlobSize)
	var secret string
	if size > 0 && stored.CredentialBlob != nil {
		blob := unsafe.Slice(stored.CredentialBlob, size)
		buffer := make([]byte, size)
		copy(buffer, blob)
		zeroBytes(blob)
		secret = string(buffer)
		zeroBytes(buffer)
	}
	procCredFree.Call(uintptr(unsafe.Pointer(stored)))
	if secret == "" {
		return "", errors.New("the Windows Credential Manager returned an empty credential")
	}
	return secret, nil
}

func remove(_ context.Context, account string) error {
	if err := validateAccount(account); err != nil {
		return err
	}
	if err := findProcs(procCredDeleteW); err != nil {
		return err
	}
	target, err := targetPointer(account)
	if err != nil {
		return err
	}
	result, _, lastErr := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(target)),
		uintptr(credTypeGeneric),
		0,
	)
	runtime.KeepAlive(target)
	return callErrno(result, lastErr, "delete the credential from the Windows Credential Manager")
}
