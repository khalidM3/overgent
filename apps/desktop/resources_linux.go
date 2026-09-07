//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// Where an installed application keeps the things it ships beside itself.
//
// There is no bundle format here, so the layout is chosen rather than dictated:
// a `resources` directory next to the executable, holding the CLI, `backend/`,
// and `icons/`. It is the one arrangement that survives all three ways this
// application is actually installed:
//
//   - An AppImage, where everything lives inside a mount that only exists while
//     the app is running, and paths under it are the only ones that resolve.
//   - A tarball extracted wherever the member chose.
//   - A package installing into /opt/overgent with /usr/bin/overgent-desktop as
//     a symlink. os.Executable reads /proc/self/exe, which is already resolved,
//     so it answers /opt/overgent/overgent-desktop and the resources are found
//     beside it rather than beside the symlink.
//
// A packaging step that splits these across /usr/bin and /usr/share would need
// its own answer here; that is why the layout is stated in one place and
// scripts/build-desktop.mjs stages exactly it.

// bundledBackendName is the Convex backend executable's filename.
const bundledBackendName = "convex-local-backend"

// managedCLIName is the filename the app installs the CLI under.
const managedCLIName = "overgent"

func bundledResourceDirectory() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		resolved = executable
	}
	return filepath.Join(filepath.Dir(resolved), "resources"), nil
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
// next update replaces - and, for an AppImage, never names a mount that stops
// existing when the app exits.
//
// ~/.local/bin is what the XDG user directory conventions and every recent
// distribution's default profile put on PATH, and it is the same directory the
// CLI's own installer uses.
func managedCLIDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	// XDG_BIN_HOME is not part of the base directory specification, but enough
	// tooling honours it that ignoring a member who has set it would install to
	// a directory they deliberately moved away from.
	if custom := strings.TrimSpace(os.Getenv("XDG_BIN_HOME")); filepath.IsAbs(custom) {
		return filepath.Clean(custom), nil
	}
	return filepath.Join(home, ".local", "bin"), nil
}
