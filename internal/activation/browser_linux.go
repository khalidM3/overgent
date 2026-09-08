//go:build linux

package activation

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// openBrowser goes through xdg-open, the desktop-agnostic handler every major
// Linux desktop installs. Looking it up first turns the common headless case
// into a message that names the missing piece, rather than "exec: not found".
func openBrowser(ctx context.Context, target string) error {
	opener, err := exec.LookPath("xdg-open")
	if err != nil {
		return errors.New("xdg-open is not installed, so Overgent cannot open your browser; open the printed URL yourself, or install xdg-utils")
	}
	return exec.CommandContext(ctx, opener, target).Run()
}

// openApp has no route yet: the overgent:// scheme is registered by the desktop
// app's installer, and the desktop app is not built for Linux. Fail closed and
// say why, rather than shelling out to a handler that is certainly absent.
func openApp(context.Context, string) error {
	return fmt.Errorf("the Overgent desktop app is not available on Linux, so it cannot open a Project; use the hosted dashboard")
}
