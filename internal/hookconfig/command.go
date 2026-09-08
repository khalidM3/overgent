package hookconfig

import (
	"errors"
	"path"
	"runtime"
	"strings"
)

// A managed hook command is one string that a vendor hands to a shell. Which
// shell that is differs by vendor and by platform, and the three shells this
// repository has to satisfy do not agree on quoting, so the generated form is
// chosen per platform and per vendor rather than assumed.
//
// The evidence behind each choice, read from shipped code rather than from
// documentation:
//
//   - Claude Code (@anthropic-ai/claude-code). A `type: "command"` hook is run
//     with `spawn(command, [], {shell})`. On Windows `shell` is the Git Bash
//     executable when Git for Windows is installed, and the settings schema
//     documents the default as "bash (powershell on Windows without Git Bash)".
//     A form that depends on whether a member happens to have Git for Windows
//     cannot round-trip, so Overgent pins `"shell": "powershell"` on the
//     handler it writes (see expected) and generates the PowerShell form.
//     Windows PowerShell ships with the operating system; Git Bash does not.
//
//   - Codex (openai/codex, codex-rs/hooks/src/engine/command_runner.rs).
//     default_shell_command resolves `%COMSPEC%` with a fallback of `cmd.exe`
//     and the argument `/C` on Windows, and the command line is appended with
//     raw_arg(format!(r#""{command_line}""#)) — that is, `cmd.exe /C "<us>"`.
//     cmd.exe has more than two quote characters to consider there, so it
//     strips the leading quote and the final quote and parses what remains,
//     which is exactly the string generated here. cmd.exe does not treat a
//     single quote as a quote character at all.
//
//   - Cursor (Cursor.app, extensions/cursor-agent-exec). The hook runner builds
//     `@'\n<json>\n'@ | <command>` when process.platform is "win32" — a
//     PowerShell single-quoted here-string piped into the hook command — and
//     its Windows shell executor writes a .ps1 and runs it through pwsh or
//     powershell.exe. PowerShell needs the call operator to run a quoted
//     command name, and escapes a single quote by doubling it.
type commandStyle int

const (
	// stylePOSIX quotes for `/bin/sh -c`: single quotes, `'\''` for a literal
	// single quote, and no call operator.
	stylePOSIX commandStyle = iota
	// stylePowerShell quotes for `powershell -Command`: single quotes, `''`
	// for a literal single quote, and a leading `&` so that a quoted command
	// name is executed rather than echoed as a string.
	stylePowerShell
	// styleCmd quotes for `cmd.exe /C`: double quotes, with the Windows argv
	// rule that a run of backslashes immediately before the closing quote is
	// doubled.
	styleCmd
)

// hookOS is the platform whose command form this package generates and pins
// into handlers. It exists as a variable only so the tests can exercise every
// platform's form from one host; nothing outside tests assigns to it.
var hookOS = runtime.GOOS

// styleFor reports how vendor parses a managed hook command on goos.
func styleFor(goos, vendor string) commandStyle {
	if goos != "windows" {
		return stylePOSIX
	}
	if vendor == "codex" {
		return styleCmd
	}
	// Claude Code is pinned to PowerShell by the handler this package writes,
	// and Cursor runs Windows hooks through PowerShell unconditionally.
	return stylePowerShell
}

const vendorMarker = " agent-hook --vendor "

// joinCommand assembles a command line from an already-quoted executable and
// the fixed arguments that follow it.
func joinCommand(style commandStyle, parts ...string) string {
	line := strings.Join(parts, " ")
	if style == stylePowerShell {
		return "& " + line
	}
	return line
}

func quoteArgument(style commandStyle, value string) string {
	switch style {
	case styleCmd:
		return `"` + value + strings.Repeat(`\`, trailingBackslashes(value)) + `"`
	case stylePowerShell:
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	default:
		return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
	}
}

func validateArgument(style commandStyle, value string) error {
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("hook command argument is empty or contains a control character")
	}
	if strings.Contains(value, vendorMarker) {
		return errors.New("hook command path contains the managed-command delimiter")
	}
	// Codex currently feeds its Windows command through cmd.exe /C. Percent is
	// expanded by cmd even inside double quotes, so a path containing it cannot
	// be represented without changing the bytes that reach Overgent. Refuse the
	// binding instead of installing a command that can execute a substituted
	// value. Delayed expansion is not enabled by Codex, so ! is inert here.
	if style == styleCmd && strings.ContainsRune(value, '%') {
		return errors.New("Codex Windows hook paths containing % are unsupported")
	}
	// A double quote is not legal in a Windows path and would terminate cmd's
	// quoted argument. Reject it here as defense in depth for direct tests and
	// for any future caller that supplies a non-filesystem argument.
	if style == styleCmd && strings.ContainsRune(value, '"') {
		return errors.New("Codex Windows hook paths containing a quote are unsupported")
	}
	return nil
}

func trailingBackslashes(value string) int {
	count := 0
	for count < len(value) && value[len(value)-1-count] == '\\' {
		count++
	}
	return count
}

const configRootArgument = "--config-root"

// parsePrefix splits the `<executable> [--config-root <root>]` head of a
// managed hook command into its two halves, undoing whichever quoting form
// wrote it. An empty configRoot means the portable form, which names no
// profile.
//
// It accepts all three forms on every host rather than only this host's. A
// settings file that reaches another platform - a synchronized home directory,
// a repository checked out on two machines - is still recognized as Overgent's
// own binding, which routes it to reconnect or removal. Refusing to parse it
// would instead report unknown managed-looking drift and leave the member with
// hooks they cannot remove.
func parsePrefix(prefix string) (executable, configRoot string, ok bool) {
	prefix = strings.TrimPrefix(prefix, "& ")
	if len(prefix) < 2 {
		return "", "", false
	}
	quote := prefix[0]
	if quote != '\'' && quote != '"' {
		return "", "", false
	}
	if prefix[len(prefix)-1] != quote {
		return "", "", false
	}
	marker := string(quote) + " " + configRootArgument + " " + string(quote)
	at := strings.Index(prefix, marker)
	if at < 0 {
		executable, ok = unquoteArgument(quote, prefix)
		return executable, "", ok
	}
	executable, ok = unquoteArgument(quote, prefix[:at+1])
	if !ok {
		return "", "", false
	}
	configRoot, ok = unquoteArgument(quote, prefix[at+len(marker)-1:])
	if !ok || configRoot == "" {
		return "", "", false
	}
	return executable, configRoot, true
}

func isAbsolutePath(value string) bool {
	return strings.HasPrefix(value, "/") || windowsAbsolute(value)
}

func pathIsAbs(value, goos string) bool {
	if goos == "windows" {
		return windowsAbsolute(value)
	}
	return strings.HasPrefix(value, "/")
}

func windowsAbsolute(value string) bool {
	if len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/') {
		return true
	}
	normalized := strings.ReplaceAll(value, `\`, "/")
	if !strings.HasPrefix(normalized, "//") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(normalized, "//"), "/")
	return len(parts) >= 2 && parts[0] != "" && parts[1] != ""
}

func cleanPortablePath(value string) string {
	if !windowsAbsolute(value) {
		return path.Clean(value)
	}
	normalized := path.Clean(strings.ReplaceAll(value, `\`, "/"))
	return strings.ReplaceAll(normalized, "/", `\`)
}

func sameProfile(left, right string) bool {
	if windowsAbsolute(left) || windowsAbsolute(right) {
		return windowsAbsolute(left) && windowsAbsolute(right) && strings.EqualFold(cleanPortablePath(left), cleanPortablePath(right))
	}
	return cleanPortablePath(left) == cleanPortablePath(right)
}

func unquoteArgument(quote byte, value string) (string, bool) {
	if len(value) < 2 || value[0] != quote || value[len(value)-1] != quote {
		return "", false
	}
	inner := value[1 : len(value)-1]
	if quote == '"' {
		// Only the run of backslashes that touches the closing quote was
		// doubled, because that is the only run Windows argument parsing reads
		// as an escape.
		trailing := trailingBackslashes(inner)
		if trailing%2 != 0 {
			return "", false
		}
		return inner[:len(inner)-trailing] + strings.Repeat(`\`, trailing/2), true
	}
	// POSIX and PowerShell escape a literal single quote differently. The two
	// rules only ever disagree for a path that contains one, and `'\''` cannot
	// be produced by PowerShell quoting of any path a Windows filesystem
	// accepts, so its presence identifies the POSIX form unambiguously.
	if strings.Contains(inner, `'\''`) {
		return strings.ReplaceAll(inner, `'\''`, "'"), true
	}
	return strings.ReplaceAll(inner, "''", "'"), true
}
