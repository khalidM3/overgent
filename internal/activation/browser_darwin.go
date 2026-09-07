//go:build darwin

package activation

import (
	"context"
	"fmt"
	"os/exec"
)

func openBrowser(ctx context.Context, target string) error {
	return exec.CommandContext(ctx, "/usr/bin/open", target).Run()
}

// openApp lets Launch Services route the scheme to the installed bundle. The
// bundle id is passed explicitly so a stale registration of a development
// build cannot claim the released app's links.
func openApp(ctx context.Context, projectID string) error {
	if err := exec.CommandContext(ctx, "/usr/bin/open", "-b", "com.overgent.app", deepLink(projectID)).Run(); err != nil {
		return fmt.Errorf("open Overgent app: %w", err)
	}
	return nil
}
