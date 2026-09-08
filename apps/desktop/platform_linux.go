//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"

	servicemanager "github.com/khalidM3/overgent/internal/service"
)

// defaultDeviceName is the label offered for a machine that reports no
// hostname.
const defaultDeviceName = "This computer"

// agentInstallCandidates lists where each vendor's installer puts its binary
// when it is not on PATH.
//
// There is no .app bundle here, so the list is shorter than the macOS one and
// is mostly about the two ways a GUI application's PATH differs from a login
// shell's: a desktop session started before ~/.local/bin existed, and nvm,
// which is a shell function that no desktop session ever sources.
func agentInstallCandidates(command, home string) []string {
	local := filepath.Join(home, ".local", "bin")
	switch command {
	case "codex":
		return []string{
			filepath.Join(local, "codex"),
			filepath.Join(home, ".codex", "bin", "codex"),
			"/usr/local/bin/codex",
			"/usr/bin/codex",
		}
	case "claude":
		candidates := []string{
			filepath.Join(local, "claude"),
			filepath.Join(home, ".npm-global", "bin", "claude"),
			"/usr/local/bin/claude",
		}
		nvm, _ := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin", "claude"))
		return append(candidates, nvm...)
	case "cursor":
		// Cursor ships as an AppImage or a tarball rather than a package, so
		// the two directories people actually extract it into are checked
		// alongside the shell command it can install.
		return []string{
			filepath.Join(local, "cursor"),
			"/usr/local/bin/cursor",
			filepath.Join(home, "Applications", "cursor", "bin", "cursor"),
			"/opt/cursor/bin/cursor",
		}
	}
	return nil
}

// executableFile reports whether a path is something this process can run.
func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// openURLWithSystemHandler hands a URL to the desktop environment.
//
// xdg-open is the portable entry point every desktop implements, and it exits
// non-zero when no handler is registered for the scheme, which is what makes a
// missing Claude Code or VS Code handler visible rather than silently reported
// as opened. It is not present on a bare system, and an error from exec is the
// same answer for the member: the link could not be opened here.
func openURLWithSystemHandler(value string) error {
	return exec.Command("xdg-open", value).Run()
}

// openURLFallbackCommand is the command shown to a member when the handler is
// missing, so it has to be the one their own shell would accept.
func openURLFallbackCommand(value string) string { return "xdg-open " + shellQuote(value) }

// serviceManagerFor builds the systemd --user manager for the current user.
//
// Unlike launchd, `systemctl --user` addresses the calling user's own manager
// and needs no uid to find it; the field is filled anyway because the manager
// uses it to locate the session bus when it is not already in the environment.
func serviceManagerFor(executable, configRoot string) (servicemanager.Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return servicemanager.Manager{}, err
	}
	return servicemanager.Manager{Executable: executable, ConfigRoot: configRoot, Home: home, UID: os.Getuid()}, nil
}
