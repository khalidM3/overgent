//go:build !darwin

package activation

import (
	"context"
	"errors"
)

func Open(context.Context, string, string) error {
	return errors.New("dashboard browser activation is currently supported only on macOS")
}
