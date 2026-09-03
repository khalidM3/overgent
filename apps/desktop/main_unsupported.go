//go:build !darwin

package main

import (
	"fmt"
	"os"
)

func main() {
	_, _ = fmt.Fprintln(os.Stderr, "Overgent desktop preview is currently validated only on macOS")
	os.Exit(1)
}
