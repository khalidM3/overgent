//go:build !windows

package hookconfig

import "os"

// ReplaceFile atomically activates a staged file in the same directory.
func ReplaceFile(staged, destination string) error {
	return os.Rename(staged, destination)
}
