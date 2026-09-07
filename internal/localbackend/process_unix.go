//go:build unix

package localbackend

import "syscall"

// signalPID sends sig to pid, mirroring syscall.Kill's semantics on the
// platforms Overgent actually ships the local backend on.
func signalPID(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

// pidAlive reports whether pid names a live process, using the conventional
// "signal 0" liveness probe.
func pidAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
