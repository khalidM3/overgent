//go:build unix

package main

import "syscall"

// The pnpm wrapper spawns the actual convex-local backend. Interrupting only
// the wrapper leaks the backend and its port, so the harness signals the whole
// process group; these two helpers are the platform-bound half of that.
func newProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// signalProcessGroup sends sig to every process in pid's group.
func signalProcessGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}
