package main

import (
	"strings"
	"testing"
)

// The desktop entry is the whole of Linux scheme registration, and it is a file
// no test on this machine will ever see a desktop environment read. Checking
// the document itself is what keeps the three lines it actually turns on -
// the MIME type, the %u, and the window-class match - from being lost in a
// refactor that nobody can run.
func TestDesktopEntryClaimsTheSchemeAndPassesTheURL(t *testing.T) {
	entry := desktopEntry("/opt/overgent/overgent-desktop", "overgent")
	for _, required := range []string{
		"[Desktop Entry]",
		"Type=Application",
		"MimeType=x-scheme-handler/" + desktopURLScheme() + ";",
		// Without %u the link launches the app with no argument and opens the
		// start page instead of the Project the link named.
		`Exec="/opt/overgent/overgent-desktop" %u`,
		// Must equal the GTK program name main_linux.go sets, or the running
		// window is not matched to this entry.
		"StartupWMClass=" + desktopEntryName(),
		"Terminal=false",
	} {
		if !strings.Contains(entry, required) {
			t.Fatalf("desktop entry is missing %q:\n%s", required, entry)
		}
	}
}

// A home directory with a space in it is ordinary, and an unquoted Exec would
// split it into two arguments and launch nothing.
func TestDesktopEntryQuotesAnExecutablePathWithSpaces(t *testing.T) {
	entry := desktopEntry("/home/a b/Applications/Overgent.AppImage", "overgent")
	if !strings.Contains(entry, `Exec="/home/a b/Applications/Overgent.AppImage" %u`) {
		t.Fatalf("unquoted Exec:\n%s", entry)
	}
	if !strings.Contains(entry, "TryExec=/home/a b/Applications/Overgent.AppImage") {
		t.Fatalf("TryExec is not the bare path:\n%s", entry)
	}
}

// The Desktop Entry specification reserves these inside a quoted argument. An
// unescaped one is not a broken icon; it is a command line the desktop expands
// before running it.
//
// Two levels of escaping apply and both are the file's own: the format treats a
// backslash in a value as an escape, so the single backslash the Exec syntax
// wants has to be written as two.
func TestDesktopEntryEscapesReservedExecCharacters(t *testing.T) {
	entry := desktopEntry(`/home/x/$(id)/a"b`+"`c`", "overgent")
	execLine := ""
	for _, line := range strings.Split(entry, "\n") {
		if strings.HasPrefix(line, "Exec=") {
			execLine = line
		}
	}
	for _, escaped := range []string{`\\$(id)`, `a\\"b`, "\\\\`c"} {
		if !strings.Contains(execLine, escaped) {
			t.Fatalf("expected %q in %s", escaped, execLine)
		}
	}
	// The closing quote must still be the one this function added, not one the
	// path smuggled in.
	if !strings.HasSuffix(execLine, `" %u`) || strings.Count(execLine, `="`) != 1 {
		t.Fatalf("Exec quoting was broken by the path: %s", execLine)
	}
}

// Windows resolves a link by running this string. Both halves are quoted so
// that neither an install path nor the URL can add an argument to it.
func TestWindowsProtocolCommandQuotesBothHalves(t *testing.T) {
	got := windowsProtocolCommand(`C:\Program Files\Overgent\overgent-desktop.exe`)
	want := `"C:\Program Files\Overgent\overgent-desktop.exe" "%1"`
	if got != want {
		t.Fatalf("protocol command = %q, want %q", got, want)
	}
	if icon := windowsProtocolIcon(`C:\x\y.exe`); icon != `"C:\x\y.exe",0` {
		t.Fatalf("protocol icon = %q", icon)
	}
}
