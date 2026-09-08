//go:build !darwin && !linux && !windows

package credential

import (
	"context"
	"runtime"
)

// Platforms with no implemented OS credential store fail closed. There is no
// file-based or passphrase-encrypted fallback and there is not going to be one
// without an ADR: a secret written under the profile root is readable by every
// process running as the member, which is the property the OS store exists to
// remove.
func errUnsupported() error {
	return &UnavailableError{
		Platform: runtime.GOOS,
		Reason:   "Overgent implements OS credential storage on macOS, Linux and Windows only",
		Remedy:   "run the Overgent service on a supported platform",
	}
}

func put(context.Context, string, string) error   { return errUnsupported() }
func get(context.Context, string) (string, error) { return "", errUnsupported() }
func remove(context.Context, string) error        { return errUnsupported() }
