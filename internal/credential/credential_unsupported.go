//go:build !darwin

package credential

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("OS credential storage is unsupported on this platform; plaintext fallback is prohibited")

func put(context.Context, string, string) error   { return errUnsupported }
func get(context.Context, string) (string, error) { return "", errUnsupported }
func remove(context.Context, string) error        { return errUnsupported }
