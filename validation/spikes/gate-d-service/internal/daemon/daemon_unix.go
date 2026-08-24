//go:build darwin || linux

package daemon

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

type instanceLock struct{ file *os.File }

func acquireLock(path string) (*instanceLock, error) {
	if err := ensureParent(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open instance lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another service instance owns lock: %w", err)
	}
	return &instanceLock{file: f}, nil
}

func (l *instanceLock) Close() error {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	return l.file.Close()
}

func listen(path string) (net.Listener, error) {
	if err := ensureParent(path); err != nil {
		return nil, err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen current-user IPC: %w", err)
	}
	return l, nil
}

func network() string { return "unix" }
