//go:build windows

package localbackend

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// signalPID terminates pid. Windows has no POSIX signal delivery, so SIGTERM
// and SIGKILL both map to the same hard termination; the local backend is a
// portability-build target only (docs/release.md), never a shipped platform,
// so a graceful-shutdown distinction is not needed here.
func signalPID(pid int, _ syscall.Signal) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.TerminateProcess(handle, 1)
}

// pidAlive reports whether pid names a still-running process.
func pidAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	// STILL_ACTIVE (259) is the Win32 exit-code sentinel for "hasn't exited yet".
	const stillActive = 259
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}
