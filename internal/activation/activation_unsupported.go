//go:build !darwin

package activation

import (
	"context"
	"errors"
)

func Open(context.Context, string, string) error {
	return errors.New("dashboard browser activation is currently supported only on macOS")
}

func OpenApp(context.Context, string) error {
	return errors.New("opening the Overgent app is currently supported only on macOS")
}
