//go:build unix

// The local IPC transport on every unix: a Unix socket inside the profile
// directory, and an flock on a sibling file so a second service cannot start.
//
// This was tagged darwin only because macOS was the one qualified platform,
// not because anything here is Apple-specific. flock, Unix sockets, and the
// 0600-inside-0700 mode that restricts the endpoint to its owner behave the
// same on Linux, so the tag is what changed rather than the code.

package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

type fileLock struct{ f *os.File }

func acquire(path string) (Lock, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0o700); e != nil {
		return nil, e
	}
	if e := os.Chmod(filepath.Dir(path), 0o700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if e != nil {
		return nil, e
	}
	if e = f.Chmod(0o600); e != nil {
		f.Close()
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("service already running: %w", e)
	}
	return &fileLock{f}, nil
}
func (l *fileLock) Close() error {
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	return l.f.Close()
}
func serve(ctx context.Context, path string, h Handler) error {
	_ = os.Remove(path)
	if e := os.MkdirAll(filepath.Dir(path), 0o700); e != nil {
		return e
	}
	if e := os.Chmod(filepath.Dir(path), 0o700); e != nil {
		return e
	}
	ln, e := net.Listen("unix", path)
	if e != nil {
		return e
	}
	defer ln.Close()
	defer os.Remove(path)
	if e = os.Chmod(path, 0o600); e != nil {
		return e
	}
	go func() { <-ctx.Done(); ln.Close() }()
	for {
		c, e := ln.Accept()
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			return e
		}
		go serveConn(ctx, c, h)
	}
}
func dial(ctx context.Context, path string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", path)
}
