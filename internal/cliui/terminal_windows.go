//go:build windows

package cliui

import "os"

// Windows support is deliberately conservative until the Windows release gate:
// character devices are interactive, while files and pipes are not.
func fileIsTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func terminalWidth(*os.File) (int, bool) { return 0, false }
