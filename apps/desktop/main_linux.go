//go:build linux

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Linux runs the same shell on WebKitGTK. Wails v3 needs CGO and the
// webkit2gtk-4.1 and libgtk-3 development headers to build it; see
// apps/desktop/README.md for the package names.
func main() {
	runShell(platformShell{
		Linux: application.LinuxOptions{
			// GTK reads the program name to group windows and match them to an
			// installed .desktop file. Without it the window is grouped under
			// the executable's filename, so the app shows up in the dock or
			// switcher with the generic icon rather than its own, even though
			// the .desktop file the installer wrote is correct. It has to equal
			// the .desktop file's basename that scripts/build-desktop.mjs
			// generates.
			ProgramName: desktopEntryName(),
		},
		LinuxWindow: application.LinuxWindow{
			// The window manager takes its minimised and switcher icon from the
			// window itself, not from the .desktop file, so it is set here too.
			Icon: overgentMarkPNG(markInk),
			// WebKitGPU acceleration on demand rather than never: the dashboard
			// is an ordinary document, but Wails defaults a nil Linux block to
			// Never, and this window is also the one that renders a live
			// Project. On-demand is the framework's own middle setting.
			WebviewGpuPolicy: application.WebviewGpuPolicyOnDemand,
		},
		SingleInstance: singleInstance,
		ApplyTrayIcon: func(tray *application.SystemTray) {
			// No template images outside macOS: the tray host composites what
			// it is given, so both inks are supplied and it picks by theme.
			tray.SetIcon(overgentMarkPNG(markInk))
			tray.SetDarkModeIcon(overgentMarkPNG(markPaper))
		},
	})
}
