//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"

	servicemanager "github.com/khalidM3/overgent/internal/service"
)

// defaultDeviceName is the label offered for a machine that reports no
// hostname.
const defaultDeviceName = "This PC"

// agentInstallCandidates lists where each vendor's installer puts its binary
// when it is not on PATH.
//
// Windows installers write per-user by default, so %LOCALAPPDATA% is where most
// of these actually are; the Program Files entries cover a machine-wide install
// by an administrator. Every candidate carries its extension, because
// executableFile below cannot fall back to a permission bit to recognise one.
func agentInstallCandidates(command, home string) []string {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = filepath.Join(home, "AppData", "Local")
	}
	programs := os.Getenv("ProgramFiles")
	if programs == "" {
		programs = `C:\Program Files`
	}
	switch command {
	case "codex":
		return []string{
			filepath.Join(local, "Programs", "codex", "codex.exe"),
			filepath.Join(home, ".codex", "bin", "codex.exe"),
			filepath.Join(home, "AppData", "Roaming", "npm", "codex.cmd"),
		}
	case "claude":
		return []string{
			filepath.Join(local, "Programs", "claude", "claude.exe"),
			filepath.Join(home, "AppData", "Roaming", "npm", "claude.cmd"),
			filepath.Join(home, ".local", "bin", "claude.exe"),
		}
	case "cursor":
		return []string{
			filepath.Join(local, "Programs", "cursor", "resources", "app", "bin", "cursor.cmd"),
			filepath.Join(programs, "cursor", "resources", "app", "bin", "cursor.cmd"),
		}
	}
	return nil
}

// executableFile reports whether a path is something this process can run.
//
// Windows has no executable permission bit: what makes a file runnable is its
// extension being in PATHEXT. Testing the mode the way the Unix files do would
// answer true for every readable file, so a data file beside a real binary
// would be launched as though it were one.
func executableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	switch filepath.Ext(path) {
	case ".exe", ".cmd", ".bat", ".com":
		return true
	}
	return false
}

// openURLWithSystemHandler hands a URL to the shell's protocol handler.
//
// `rundll32 url.dll,FileProtocolHandler` is used rather than `cmd /c start`
// because it takes the URL as an argument to a known entry point instead of as
// a token a command interpreter parses - a deep link is attacker-reachable, and
// `start` would treat characters inside it as shell syntax.
func openURLWithSystemHandler(value string) error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", value).Run()
}

// openURLFallbackCommand is the command shown to a member when the handler is
// missing, so it has to be the one their own shell would accept. PowerShell is
// the shell a Windows member is most likely to have open, and Start-Process
// takes the URL as one argument rather than parsing it.
func openURLFallbackCommand(value string) string {
	return "Start-Process " + powerShellQuote(value)
}

// powerShellQuote wraps a value as a PowerShell single-quoted string, where the
// only special character is the quote itself.
func powerShellQuote(value string) string {
	quoted := ""
	for _, char := range value {
		if char == '\'' {
			quoted += "''"
			continue
		}
		quoted += string(char)
	}
	return "'" + quoted + "'"
}

// webviewUserDataPath is where WebView2 keeps this build's browser profile:
// cookies, local storage, and its cache.
//
// It is named explicitly rather than left to WebView2's default of
// %APPDATA%\<executable name>, so that the profile follows the build profile
// the same way the config root and the URL scheme already do - and so that
// renaming the executable does not silently abandon a member's session.
func webviewUserDataPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			// Empty hands the decision back to WebView2's own default, which is
			// still a working application; refusing to start would not be.
			return ""
		}
		base = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(base, "Overgent", desktopEntryName(), "WebView2")
}

// serviceManagerFor builds the per-user background service manager.
//
// INTEGRATOR: lane B's internal/service Windows manager identifies the account
// with a `User string` field rather than the `UID int` this lane's base has, and
// refuses to install while it is empty. That field is not set here because
// setting it would not compile against internal/service as it stands on this
// branch. After lane B merges, add `manager.User = account.Username` below and
// drop the guard; until then this fails closed with the message rather than
// registering a scheduled task under no account at all.
func serviceManagerFor(executable, configRoot string) (servicemanager.Manager, error) {
	account, err := user.Current()
	if err != nil {
		return servicemanager.Manager{}, fmt.Errorf("resolve current user: %w", err)
	}
	if account.Username == "" || !filepath.IsAbs(account.HomeDir) {
		return servicemanager.Manager{}, errors.New("current user has no name or home directory")
	}
	return servicemanager.Manager{Executable: executable, ConfigRoot: configRoot, Home: account.HomeDir}, nil
}
