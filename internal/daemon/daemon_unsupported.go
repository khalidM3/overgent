//go:build !unix && !windows

package daemon

import (
	"context"
	"errors"
	"net"
)

// Every platform Overgent does not build for. The unix and windows files cover
// the three release targets; this keeps the package compiling anywhere else
// without pretending the service can run there.
var errUnsupportedPlatform = errors.New("local service IPC is unsupported on this platform")

func acquire(string) (Lock, error) { return nil, errUnsupportedPlatform }

func serve(context.Context, string, Handler) error { return errUnsupportedPlatform }

func dial(context.Context, string) (net.Conn, error) { return nil, errUnsupportedPlatform }
