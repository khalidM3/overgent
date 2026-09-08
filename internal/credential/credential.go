// Package credential stores Overgent's secrets in the operating system's own
// credential store: the hosted device credential, and the local backend's
// instance secret and deployment secrets key (docs/security-privacy.md,
// "Local"). Every platform is reached through the same unexported
// put/get/remove trio, so callers never branch on GOOS.
//
// There is deliberately no file-based or passphrase-encrypted fallback. When
// no OS store is reachable the platform implementation returns an
// *UnavailableError naming what is missing and what the member should do; it
// never degrades to writing a secret under the profile root.
package credential

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const serviceName = "com.overgent.comice"

// ErrNotFound reports that the OS credential store was reachable but holds no
// item for the account. Callers that treat "no credential yet" as an ordinary
// state (first run, after a revoke) can test for it with errors.Is rather than
// matching error text.
//
// The macOS implementation shells out to the Security CLI and cannot classify
// its exit status reliably, so it returns an unclassified error instead. Treat
// any error from Get as "no usable credential"; treat ErrNotFound as the
// stronger, positively identified case.
var ErrNotFound = errors.New("no credential is stored for this account")

// UnavailableError reports that this machine exposes no usable OS credential
// store. It is deliberately distinct from an ordinary read or write failure:
// the remedy is to install or start a credential store, not to retry.
//
// Reason and Remedy are written for a member reading a CLI error, and neither
// ever carries a secret value.
type UnavailableError struct {
	// Platform is the GOOS the message was written for.
	Platform string
	// Reason names what is missing, in the member's terms.
	Reason string
	// Remedy is the concrete next step.
	Remedy string
	// Cause is the underlying transport error, if there was one.
	Cause error
}

func (e *UnavailableError) Error() string {
	message := fmt.Sprintf("no OS credential store is available on %s: %s", e.Platform, e.Reason)
	if e.Remedy != "" {
		message += "; " + e.Remedy
	}
	// Say this in the error itself. A member who hits this will otherwise ask
	// for the "just write it to a file" flag, and there is not going to be one.
	message += ". Storing the secret in plaintext instead is prohibited"
	if e.Cause != nil {
		message += ": " + e.Cause.Error()
	}
	return message
}

func (e *UnavailableError) Unwrap() error { return e.Cause }

func Put(ctx context.Context, account, secret string) error   { return put(ctx, account, secret) }
func Get(ctx context.Context, account string) (string, error) { return get(ctx, account) }
func Delete(ctx context.Context, account string) error        { return remove(ctx, account) }

// validatePut rejects an item that cannot round-trip identically on every
// supported platform, before any of them is contacted.
func validatePut(account, secret string) error {
	if account == "" || secret == "" {
		return errors.New("credential account and secret are required")
	}
	if err := validateAccount(account); err != nil {
		return err
	}
	// macOS drives its store through a line-oriented PTY password prompt, so a
	// CR or LF in the secret would truncate it there. Rejecting it everywhere
	// keeps one stored item byte-identical across platforms rather than
	// silently better on Linux and Windows.
	if strings.ContainsAny(secret, "\r\n") {
		return errors.New("credential secret must not contain a carriage return or newline")
	}
	return nil
}

// validateAccount rejects an account name that could break out of the
// line-oriented and attribute-keyed lookups the platforms use.
func validateAccount(account string) error {
	if account == "" {
		return errors.New("credential account is required")
	}
	if strings.ContainsAny(account, "\r\n") {
		return errors.New("credential account must not contain a carriage return or newline")
	}
	return nil
}

// windowsMaxTargetNameLength is CRED_MAX_GENERIC_TARGET_NAME_LENGTH: the
// longest TargetName CredWriteW accepts for a CRED_TYPE_GENERIC credential.
const windowsMaxTargetNameLength = 32767

// windowsTargetName builds the Credential Manager TargetName for an account.
//
// The format is "<service>:<account>", the same two facts macOS keys on (-s
// and -a) and the same two the Secret Service keys on ("service" and
// "account"), joined with the separator the Windows ecosystem already uses for
// namespaced generic credentials (Git Credential Manager writes "git:https://
// host"). The colon cannot appear in serviceName, so the split point is
// unambiguous and an account containing a colon still maps to exactly one
// target.
//
// This lives in the platform-neutral file on purpose: it is pure string work,
// and keeping it here means its test runs on a macOS developer machine instead
// of only on a Windows runner.
func windowsTargetName(account string) string {
	return serviceName + ":" + account
}

// zeroBytes overwrites a buffer that held secret material. It is used by the
// Linux and Windows implementations; the string a caller finally receives is
// immutable and cannot be wiped, so this bounds the exposure of the
// intermediate copies only.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
