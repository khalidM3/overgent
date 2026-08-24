//go:build !darwin

package credential

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("OS credential store is unsupported by this Gate D spike; plaintext fallback is prohibited")

func Put(context.Context, string, string) error   { return errUnsupported }
func Get(context.Context, string) (string, error) { return "", errUnsupported }
func Delete(context.Context, string) error        { return errUnsupported }
