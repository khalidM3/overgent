//go:build windows

package main

import (
	"os"
	"path/filepath"
)

// Where an installed application keeps the things it ships beside itself: a
// `resources` directory next to the executable, holding the CLI, `backend\`,
// and `icons\`.
//
// The install is per user, under %LOCALAPPDATA%\Programs, which is what the
// rest of this port already assumes: the scheme is registered in HKCU, the
// background service is a per-user task, and neither would work from a
// machine-wide install without asking for elevation the member did not come
// here to grant.

// bundledBackendName is the Convex backend executable's filename. The pinned
// Convex release ships the Windows asset as convex-local-backend.exe, and the
// name has to match what scripts/fetch-backend.mjs unpacks.
const bundledBackendName = "convex-local-backend.exe"

// managedCLIName is the filename the app installs the CLI under. The extension
// is not decoration here: executableFile recognises a runnable file by it, and
// so does the shell that a managed hook invokes.
const managedCLIName = "overgent.exe"

func bundledResourceDirectory() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(absolute), "resources"), nil
}

// bundledCLIPath is the CLI copy this build carries.
func bundledCLIPath() (string, error) {
	directory, err := bundledResourceDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, managedCLIName), nil
}

// managedCLIDirectory is the stable path the bundled CLI is installed to, so a
// hook or MCP entry never names a path inside an application directory that the
// next update replaces.
//
// %LOCALAPPDATA%\Overgent\bin rather than a directory under the application, so
// that uninstalling or replacing the app does not pull the CLI out from under a
// coding agent that is mid-session. Nothing puts it on PATH; every managed
// binding names the absolute path, which is the same contract as on the other
// two platforms.
func managedCLIDirectory() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(base, "Overgent", "bin"), nil
}
