//go:build windows

package activation

import (
	"context"
	"fmt"
	"os/exec"
)

// openBrowser goes through the shell's protocol handler rather than "cmd /c
// start", whose first quoted argument is taken as a window title and whose
// ampersand handling is its own hazard. rundll32 takes the URL as one argument
// and hands it to the registered handler unchanged.
func openBrowser(ctx context.Context, target string) error {
	return exec.CommandContext(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", target).Run()
}

// openApp has no route yet: the overgent:// scheme is registered under
// HKCU\Software\Classes by the desktop app's installer, and the desktop app is
// not built for Windows.
func openApp(context.Context, string) error {
	return fmt.Errorf("the Overgent desktop app is not available on Windows, so it cannot open a Project; use the hosted dashboard")
}
