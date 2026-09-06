//go:build windows

package cliui

import (
	"errors"
	"os"
)

func makeRaw(*os.File) (func(), error) {
	return nil, errors.New("interactive selection is not yet supported on Windows")
}
func inputReady(*os.File, int) bool { return false }
