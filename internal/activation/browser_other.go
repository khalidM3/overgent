//go:build !darwin && !linux && !windows

package activation

import (
	"context"
	"errors"
)

func openBrowser(context.Context, string) error {
	return errors.New("opening a browser is unsupported on this platform")
}

func openApp(context.Context, string) error {
	return errors.New("the Overgent desktop app is unsupported on this platform")
}
