//go:build windows

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Windows runs the same shell on WebView2, which the Evergreen runtime provides
// and which the installer is responsible for ensuring is present.
func main() {
	runShell(platformShell{
		Windows: application.WindowsOptions{
			// A distinct window class keeps this application's windows out of
			// the default WailsWebviewWindow class, which is what a shortcut's
			// AppUserModelID and the taskbar use to group windows.
			WndClass: "OvergentWebviewWindow",
			// WebView2 keeps a user-data directory per application. Left empty
			// it lands in %APPDATA%\<binary name>, which would make the release
			// build and a development build with a different executable name
			// silently share nothing - and would also put it under a name that
			// changes when the executable is renamed. Naming it explicitly ties
			// the browser profile to the build profile, matching how the
			// config root and the scheme are already separated.
			WebviewUserDataPath: webviewUserDataPath(),
		},
		WindowsWindow: application.WindowsWindow{
			// Follow the system's light/dark setting, the same as the page
			// inside it does.
			Theme: application.SystemDefault,
		},
		SingleInstance: singleInstance,
		ApplyTrayIcon: func(tray *application.SystemTray) {
			tray.SetIcon(overgentMarkPNG(markInk))
			tray.SetDarkModeIcon(overgentMarkPNG(markPaper))
		},
	})
}
