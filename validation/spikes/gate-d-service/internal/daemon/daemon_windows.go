//go:build windows

package daemon

import (
	"errors"
	"net"
)

type instanceLock struct{}

func acquireLock(string) (*instanceLock, error) {
	return nil, errors.New("Windows named-pipe instance locking is not validated in Gate D")
}
func (*instanceLock) Close() error { return nil }
func listen(string) (net.Listener, error) {
	return nil, errors.New("Windows named-pipe IPC is not validated in Gate D")
}
func network() string { return "tcp" }
