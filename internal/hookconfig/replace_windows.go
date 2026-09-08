//go:build windows

package hookconfig

import "golang.org/x/sys/windows"

// ReplaceFile atomically activates a staged file in the same directory.
// os.Rename cannot replace an existing regular file on Windows.
func ReplaceFile(staged, destination string) error {
	from, err := windows.UTF16PtrFromString(staged)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
