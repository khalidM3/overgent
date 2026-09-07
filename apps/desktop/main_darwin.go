//go:build darwin

package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	runShell(platformShell{
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyRegular,
		},
		MacWindow: application.MacWindow{
			// One bar, and it is the app's own. A separate native title bar spent
			// a strip of every window on a word the sidebar already says, and put
			// a system-drawn edge between the window frame and an interface whose
			// whole look is hairlines. Hidden-inset keeps the traffic lights where
			// macOS puts them and hands the rest of that strip to the page; the
			// sidebar reserves the top-left corner for them and marks the strip
			// draggable (`--wails-draggable`, style.css "desktop shell"), so the
			// window still moves by its top edge.
			TitleBar:    application.MacTitleBarHiddenInset,
			TabbingMode: application.MacWindowTabbingModeDisallowed,
			WebviewPreferences: application.MacWebviewPreferences{
				// This window navigates to the hosted origin to show a live
				// Project, and a hosted page has no native bridge to ask where
				// it is running. The name is appended to the webview's user
				// agent, so that page can still tell it is inside the app and
				// say "continue on this Mac" instead of offering to open the
				// app the member is already looking at. It grants nothing: the
				// bridge stays unreachable from any origin but this one.
				ApplicationNameForUserAgent: desktopUserAgentName,
			},
		},
		// No SingleInstance. Launch Services owns the overgent:// scheme and
		// re-activates the running application rather than starting a second
		// one, so the second-instance machinery the other platforms need would
		// never fire here - and installing it would add a lock file to a
		// platform that has never needed one.
		ApplyTrayIcon: func(tray *application.SystemTray) {
			tray.SetTemplateIcon(menuBarIcon())
		},
	})
}

// menuBarIcon is the mark as a macOS template image: pure black with an alpha
// channel, which is what lets macOS tint it for the menu bar's own appearance,
// including while a menu is open and under Reduce Transparency. Supplying real
// colours here is what produces the washed-out icon that never matches its
// neighbours.
func menuBarIcon() []byte { return overgentMarkPNG(markInk) }
