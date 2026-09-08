//go:build darwin

package main

import (
	"os"
	"path/filepath"
)

// Where an installed application keeps the things it ships beside itself.
//
// On macOS that is Contents/Resources inside the .app bundle, which is the one
// place a signed bundle may carry an executable and still verify. The layout is
// fixed by scripts/build-desktop.mjs, so this and the packaging step have to
// agree; the Linux and Windows files answer the same two questions for their
// own install layouts.

// bundledBackendName is the Convex backend executable's filename inside the
// resource directory. Windows adds .exe; nothing else varies.
const bundledBackendName = "convex-local-backend"

// managedCLIName is the filename the app installs the CLI under.
const managedCLIName = "overgent"

// bundledResourceDirectory is the directory holding everything the app ships:
// the CLI beside the app, and the backend/ subdirectory under it.
func bundledResourceDirectory() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	// .../Overgent.app/Contents/MacOS/overgent-desktop -> .../Contents/Resources
	return filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Resources")), nil
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
// hook or MCP entry never names a path inside an app bundle that the next
// update replaces. ~/.local/bin is already on PATH for most shells and is the
// same directory the installer script uses.
func managedCLIDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}
