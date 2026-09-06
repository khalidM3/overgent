//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cliui

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func makeRaw(file *os.File) (func(), error) {
	fd := int(file.Fd())
	original, err := unix.IoctlGetTermios(fd, terminalGetAttributeRequest)
	if err != nil {
		return nil, fmt.Errorf("read terminal mode: %w", err)
	}
	raw := *original
	raw.Iflag &^= unix.BRKINT | unix.ICRNL | unix.INPCK | unix.ISTRIP | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Cflag |= unix.CS8
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.IEXTEN | unix.ISIG
	raw.Cc[unix.VMIN], raw.Cc[unix.VTIME] = 1, 0
	if err = unix.IoctlSetTermios(fd, terminalSetAttributeRequest, &raw); err != nil {
		return nil, fmt.Errorf("enter terminal selection mode: %w", err)
	}
	return func() { _ = unix.IoctlSetTermios(fd, terminalSetAttributeRequest, original) }, nil
}

func inputReady(file *os.File, milliseconds int) bool {
	descriptors := []unix.PollFd{{Fd: int32(file.Fd()), Events: unix.POLLIN}}
	ready, err := unix.Poll(descriptors, milliseconds)
	return err == nil && ready > 0 && descriptors[0].Revents&unix.POLLIN != 0
}
