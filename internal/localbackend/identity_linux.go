//go:build linux

package localbackend

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// processMatches reports whether pid is a live process of this user running a
// command with this base name. It is what stops killStale from signalling a
// recycled pid that now belongs to something else.
//
// Linux reads this out of procfs rather than out of ps, because ps prints comm
// from the kernel's TASK_COMM_LEN field, which is sixteen bytes including the
// terminator. "convex-local-backend" is twenty characters, so ps reports
// "convex-local-ba" and the comparison never matches. The visible symptom is
// not a wrong answer, it is silence: killStale concludes the pid belongs to
// something else, declines to signal it, and a backend this profile started
// and lost track of keeps running and keeps holding its port. The next start
// then moves to a new port (2026-09-07).
//
// /proc/<pid>/exe is the resolved path of the running image and has no such
// limit. Every Linux that can run the service has procfs mounted; systemd
// requires it.
func processMatches(pid int, name string) bool {
	if !pidAlive(pid) {
		return false
	}
	directory := "/proc/" + strconv.Itoa(pid)
	// Ownership first. /proc/<pid>/status is world-readable, while the exe
	// link is not, so checking the uid first keeps another user's process a
	// clean "no" rather than an unreadable one.
	uid, ok := procStatusUID(directory)
	if !ok || uid != os.Getuid() {
		return false
	}
	if target, err := os.Readlink(directory + "/exe"); err == nil {
		// A replaced or deleted image reads back with this suffix appended.
		return filepath.Base(strings.TrimSuffix(target, " (deleted)")) == name
	}
	// The exe link can be refused even for one's own process under hardened
	// procfs (hidepid) or for a setuid image. argv[0] is readable there and is
	// also untruncated, so it is a sound second source rather than a guess.
	command, err := os.ReadFile(directory + "/cmdline")
	if err != nil {
		return false
	}
	argv0, _, _ := bytes.Cut(command, []byte{0})
	if len(argv0) == 0 {
		return false
	}
	return filepath.Base(string(argv0)) == name
}

// procStatusUID returns the real uid recorded for a process. The Uid line
// carries four values (real, effective, saved, filesystem); the real uid is
// the one that answers "did this profile start it".
func procStatusUID(directory string) (int, bool) {
	status, err := os.ReadFile(directory + "/status")
	if err != nil {
		return 0, false
	}
	for line := range strings.SplitSeq(string(status), "\n") {
		rest, found := strings.CutPrefix(line, "Uid:")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return 0, false
		}
		uid, convErr := strconv.Atoi(fields[0])
		if convErr != nil {
			return 0, false
		}
		return uid, true
	}
	return 0, false
}
