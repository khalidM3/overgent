//go:build windows

package daemon

import (
	"context"
	"errors"
	"net"
)

// Windows has neither Unix sockets nor flock, so the transport here is a named
// pipe plus LockFileEx rather than a retag of the unix file.
//
// These stubs fail closed until that lands. The endpoint name is already
// resolved for this platform — config.endpointFor returns a \\.\pipe\ name on
// Windows — so an implementation plugs in behind these three functions without
// touching the configuration or the callers.
//
// The security property to reproduce is the one the unix side gets from the
// filesystem: a 0600 socket in a 0700 directory is reachable only by the owning
// user. A pipe inherits no such protection, so it has to be created with a
// security descriptor granting the current user's SID alone.
var errWindowsIPC = errors.New("local service IPC is not yet implemented on Windows")

func acquire(string) (Lock, error) { return nil, errWindowsIPC }

func serve(context.Context, string, Handler) error { return errWindowsIPC }

func dial(context.Context, string) (net.Conn, error) { return nil, errWindowsIPC }
