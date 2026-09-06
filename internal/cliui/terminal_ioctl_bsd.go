//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package cliui

import "golang.org/x/sys/unix"

const terminalGetAttributeRequest = unix.TIOCGETA
const terminalSetAttributeRequest = unix.TIOCSETA
