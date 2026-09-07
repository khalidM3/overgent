package main

import "strings"

// The registration a platform writes for the overgent:// scheme is one small
// document per OS - a desktop entry, or a pair of registry strings. Building
// that document is a pure function of the executable path and this build's
// identity, so it lives here, where a test can read it on any machine. Only
// putting it where the OS looks is platform-bound, and that is all the
// deeplink_register_*.go files do.

// desktopEntry is the .desktop file contents.
//
// MimeType is the line that makes this a scheme handler. StartupWMClass has to
// equal the GTK program name main_linux.go sets, or the running window is not
// matched to this entry and appears in the switcher as a second, unnamed
// application beside its own icon.
func desktopEntry(executable string) string {
	return strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Version=1.0",
		"Name=" + desktopProductName(),
		"Comment=Persistent coordination for teams working with coding agents",
		// %u passes the URL the link carried. Without it the application is
		// launched with no argument and the link silently opens the app on its
		// start page instead of the Project it named.
		"Exec=" + quoteDesktopExec(executable) + " %u",
		"TryExec=" + executable,
		"Icon=" + desktopEntryName(),
		"Terminal=false",
		"Categories=Development;Utility;",
		"MimeType=x-scheme-handler/" + desktopURLScheme() + ";",
		"StartupWMClass=" + desktopEntryName(),
		"StartupNotify=true",
		"",
	}, "\n")
}

// quoteDesktopExec applies the Desktop Entry specification's quoting to the
// program path. The reserved characters are escaped inside a quoted string
// rather than the path being interpolated raw, because a home directory with a
// space in it is ordinary and would otherwise split into two arguments.
func quoteDesktopExec(value string) string {
	replacer := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", "$", `\\$`)
	return `"` + replacer.Replace(value) + `"`
}

// windowsProtocolCommand is the command line the shell runs for a link.
//
// %1 is the URL. The executable path and the URL are quoted separately so that
// neither a space in an install path nor one in a link can append an argument
// to the command line; the shell substitutes %1 literally inside its quotes.
func windowsProtocolCommand(executable string) string {
	return `"` + executable + `" "%1"`
}

// windowsProtocolIcon names the icon the shell shows for the scheme.
//
// The packaging step stages an .ico beside the application rather than
// embedding one in the executable, which would need a resource-compiled .syso
// at build time. So the file is named directly when it is there, and the
// executable's own resources - index 0, where an embedded icon would be - are
// the fallback. Naming a missing file would show the generic unknown-program
// icon on every link.
func windowsProtocolIcon(executable, iconPath string) string {
	if iconPath != "" {
		return `"` + iconPath + `",0`
	}
	return `"` + executable + `",0`
}
