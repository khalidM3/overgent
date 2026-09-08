//go:build windows

package main

import (
	"os"
	"syscall"
)

// A new process group insulates the backend from a Ctrl+Break aimed at the
// harness, mirroring Setpgid on unix.
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// signalProcessGroup terminates the process. Windows has no POSIX signal
// delivery and no way to signal a group by negative pid, so the graceful and
// forceful paths collapse into the same hard termination. This harness is a
// development tool that has only ever been run on macOS, so the distinction
// costs nothing here; what matters is that the package builds for windows.
func signalProcessGroup(pid int, _ syscall.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}
