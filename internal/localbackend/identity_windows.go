//go:build windows

package localbackend

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// processMatches reports whether pid is a live process this user can act on,
// running an image with this base name.
//
// The Unix version answers this with /bin/ps and a uid comparison, neither of
// which exists here. Opening the process is the Windows equivalent of the
// ownership half: a process belonging to another account refuses
// PROCESS_QUERY_LIMITED_INFORMATION unless the caller holds SeDebugPrivilege,
// which Overgent never asks for. The image name is then the same check the
// Unix side makes against comm, case-insensitively because Windows paths are.
func processMatches(pid int, name string) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	// Long paths are not capped at MAX_PATH, and a truncated image path would
	// silently compare unequal, so the buffer is the full path maximum.
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return false
	}
	return strings.EqualFold(filepath.Base(windows.UTF16ToString(buffer[:size])), name)
}
