//go:build aix || linux || solaris

package cliui

import "golang.org/x/sys/unix"

const terminalGetAttributeRequest = unix.TCGETS
const terminalSetAttributeRequest = unix.TCSETS
