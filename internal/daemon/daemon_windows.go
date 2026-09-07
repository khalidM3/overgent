//go:build windows

// The local IPC transport on Windows: a named pipe restricted to the owning
// user by an explicit DACL, and a LockFileEx byte-range lock on a sibling file
// so a second service cannot start.
//
// Neither unix primitive exists here, so what carries over is the behaviour
// rather than the mechanism:
//
//   - Access control. Unix gets it from the filesystem — a 0600 socket inside a
//     0700 directory is reachable only by the uid that owns it. A named pipe is
//     a kernel object, not a filesystem object, so it inherits nothing from the
//     profile directory and defaults to a descriptor that admits far more than
//     its creator. The equivalent has to be stated explicitly: a protected DACL
//     carrying exactly one ACE, for the SID this process runs as.
//
//   - Single instance. Unix gets it from flock(LOCK_EX|LOCK_NB). LockFileEx
//     with LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY has the same two
//     properties that matter: it refuses rather than waits, and it conflicts
//     with a second handle even inside the same process.
//
// The Windows endpoint name is already resolved by config.endpointFor, which
// returns a \\.\pipe\overgent-<16 hex> name derived from the profile root. This
// file consumes that value as-is and never constructs a name of its own.

package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// dialTimeout bounds a connect attempt when the caller supplied no deadline.
//
// DialPipeContext fails immediately when the pipe does not exist, which is the
// case that matters and matches a unix dial to an absent socket. It retries for
// as long as the context allows in one other case: the pipe exists but every
// instance is busy. Most callers hand Call a context with no deadline, and Call
// applies its own three seconds only after dial has returned, so without a
// bound here a saturated service would hang the CLI instead of failing it.
// Three seconds is the same budget Call spends on the request itself.
const dialTimeout = 3 * time.Second

// lockAll is the byte range acquire locks: the entire 64-bit span, so the lock
// covers the file rather than a region of it. The Go toolchain's own lockedfile
// helper locks the same way. Both contenders asking for the identical range is
// what makes the second request conflict instead of slotting in beside the
// first.
const lockAll = ^uint32(0)

// ownerOnlySecurityDescriptor builds the SDDL the pipe is created with.
//
// "D:" opens a DACL, "P" marks it protected so no ACE can be inherited into it,
// and the single "A" ACE allows "GA" (GENERIC_ALL) to the SID this process's
// token names as its user. Nothing else appears: no Everyone (WD), no
// Authenticated Users (AU), and above all not the absent DACL that Windows
// reads as a grant of full control to everyone — the NULL DACL is the Windows
// spelling of chmod 0666, and it is what makes a careless pipe a local
// privilege boundary failure.
//
// GENERIC_ALL rather than something narrower because both ends of this pipe are
// the same principal. The service needs FILE_CREATE_PIPE_INSTANCE to open each
// further instance of its own pipe and the client needs read and write; there
// is no second account to grant a reduced set to, so splitting the ACE would
// buy nothing.
func ownerOnlySecurityDescriptor() (string, error) {
	// GetCurrentProcessToken returns a pseudo-handle that must not be closed.
	user, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return "", fmt.Errorf("read current user SID: %w", e)
	}
	return "D:P(A;;GA;;;" + user.User.Sid.String() + ")", nil
}

type fileLock struct{ f *os.File }

func acquire(path string) (Lock, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0o700); e != nil {
		return nil, e
	}
	// No chmod pass to match the unix one. A Windows file mode carries only the
	// read-only attribute, so chmod 0700 here would assert a protection it does
	// not apply. The profile root lives under the user's own LOCALAPPDATA,
	// whose ACL is already user-only, and the endpoint that actually accepts
	// commands is protected by the descriptor serve creates it with.
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if e != nil {
		return nil, e
	}
	if e = windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, lockAll, lockAll, new(windows.Overlapped),
	); e != nil {
		f.Close()
		return nil, fmt.Errorf("service already running: %w", e)
	}
	return &fileLock{f}, nil
}

func (l *fileLock) Close() error {
	// Closing the handle releases the lock on its own, but unlocking first
	// mirrors the unix path and keeps the release explicit rather than a side
	// effect of teardown ordering.
	_ = windows.UnlockFileEx(windows.Handle(l.f.Fd()), 0, lockAll, lockAll, new(windows.Overlapped))
	return l.f.Close()
}

func serve(ctx context.Context, name string, h Handler) error {
	sd, e := ownerOnlySecurityDescriptor()
	if e != nil {
		return e
	}
	// Nothing corresponds to the unix os.Remove of a stale socket. A pipe stops
	// existing when the last handle to it closes, so a crashed service leaves
	// no name behind to clear; and ListenPipe creates the first instance with
	// FILE_CREATE, so if a live service still holds the name this fails outright
	// rather than quietly stealing the endpoint the way a unix rebind would.
	ln, e := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: sd})
	if e != nil {
		return fmt.Errorf("listen on %s: %w", name, e)
	}
	defer ln.Close()
	go func() { <-ctx.Done(); ln.Close() }()
	for {
		c, e := ln.Accept()
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return e
		}
		go serveConn(ctx, c, h)
	}
}

func dial(ctx context.Context, name string) (net.Conn, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, dialTimeout)
		defer cancel()
	}
	// Cancelling once this returns is safe: the context bounds the connect
	// attempt only and the connection does not retain it, so the deadline Call
	// sets afterwards is what governs the exchange — the same division of
	// labour net.Dialer.DialContext has on unix.
	return winio.DialPipeContext(ctx, name)
}
