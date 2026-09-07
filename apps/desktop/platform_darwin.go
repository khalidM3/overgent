//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"

	servicemanager "github.com/khalidM3/overgent/internal/service"
)

// defaultDeviceName is the label offered for a machine that reports no
// hostname.
const defaultDeviceName = "This Mac"

// agentInstallCandidates lists where each vendor's installer puts its binary
// when it is not on PATH.
//
// Codex and Cursor ship inside .app bundles whose CLI is only linked onto PATH
// from a menu item inside the app, so a perfectly working install is routinely
// invisible to exec.LookPath. Claude installs to a plain bin directory, but nvm
// puts one per Node version and none of them are on a GUI application's PATH.
func agentInstallCandidates(command, home string) []string {
	switch command {
	case "codex":
		return []string{
			filepath.Join(home, ".local", "bin", "codex"),
			filepath.Join(home, ".codex", "bin", "codex"),
			filepath.Join(home, "Applications", "Codex.app", "Contents", "Resources", "codex"),
			filepath.Join(home, "Applications", "ChatGPT.app", "Contents", "Resources", "codex"),
			"/Applications/Codex.app/Contents/Resources/codex",
			"/Applications/ChatGPT.app/Contents/Resources/codex",
		}
	case "claude":
		candidates := []string{
			filepath.Join(home, ".local", "bin", "claude"),
			filepath.Join(home, ".npm-global", "bin", "claude"),
		}
		nvm, _ := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "*", "bin", "claude"))
		return append(candidates, nvm...)
	case "cursor":
		return []string{
			filepath.Join(home, "Applications", "Cursor.app", "Contents", "Resources", "app", "bin", "cursor"),
			"/Applications/Cursor.app/Contents/Resources/app/bin/cursor",
		}
	}
	return nil
}

// executableFile reports whether a path is something this process can run.
func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// openURLWithSystemHandler hands a URL to Launch Services.
//
// `open` returns an error when no handler is registered for the scheme, and
// waiting for that result is what makes handler absence visible rather than
// silently reported as success.
func openURLWithSystemHandler(value string) error {
	return exec.Command("open", value).Run()
}

// openURLFallbackCommand is the command shown to a member when the handler is
// missing, so it has to be the one their own shell would accept.
func openURLFallbackCommand(value string) string { return "open " + shellQuote(value) }

// serviceManagerFor builds the LaunchAgent manager for the current user.
//
// launchd addresses a per-user agent by the GUI domain of a numeric uid, so the
// uid is resolved here and validated before it reaches launchctl.
func serviceManagerFor(executable, configRoot string) (servicemanager.Manager, error) {
	account, err := user.Current()
	if err != nil {
		return servicemanager.Manager{}, fmt.Errorf("resolve current user: %w", err)
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil || uid <= 0 || !filepath.IsAbs(account.HomeDir) {
		return servicemanager.Manager{}, errors.New("current user has invalid home or uid")
	}
	return servicemanager.Manager{Executable: executable, ConfigRoot: configRoot, Home: account.HomeDir, UID: uid}, nil
}
