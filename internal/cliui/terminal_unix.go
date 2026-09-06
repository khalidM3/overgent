//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cliui

import (
	"os"

	"golang.org/x/sys/unix"
)

func fileIsTerminal(file *os.File) bool {
	_, err := unix.IoctlGetTermios(int(file.Fd()), terminalGetAttributeRequest)
	return err == nil
}

func terminalWidth(file *os.File) (int, bool) {
	size, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	if err != nil || size.Col == 0 {
		return 0, false
	}
	return int(size.Col), true
}
