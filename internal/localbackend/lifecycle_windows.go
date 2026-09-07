//go:build windows

package localbackend

import "syscall"

// newProcessGroupAttr is Windows' nearest equivalent to Setpgid: a new
// process group insulates the backend from a Ctrl+Break aimed at its parent.
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
