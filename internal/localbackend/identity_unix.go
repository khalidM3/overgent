//go:build unix

package localbackend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// processMatches reports whether pid is a live process of this user running a
// command with this base name. It is what stops killStale from signalling a
// recycled pid that now belongs to something else.
func processMatches(pid int, name string) bool {
	if !pidAlive(pid) {
		return false
	}
	out, err := exec.Command("/bin/ps", "-o", "uid=,comm=", "-p", fmt.Sprint(pid)).Output()
	if err != nil {
		return false
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return false
	}
	if fields[0] != fmt.Sprint(os.Getuid()) {
		return false
	}
	return filepath.Base(strings.Join(fields[1:], " ")) == name
}
