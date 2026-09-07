package service

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// scopedLabel names the job a Manager owns, scoped to the profile it manages.
//
// Without this a development build using an isolated OVERGENT_CONFIG_ROOT and a
// production install both claim the same name: they overwrite each other's job
// definition and only one of them can ever be registered. The default profile
// keeps the unscoped base name so upgrading does not orphan a job that is
// already installed and running.
//
// The scheme is shared by every platform so the three implementations cannot
// drift: base for the default profile, base+separator+8 hex digits of the
// config root's SHA-256 otherwise. Only the separator differs, because launchd
// labels are dotted reverse-DNS while systemd unit names and Task Scheduler
// task names are not.
func scopedLabel(base, separator, configRoot, defaultConfigRoot string) string {
	if configRoot == "" || sameProfile(configRoot, defaultConfigRoot) {
		return base
	}
	sum := sha256.Sum256([]byte(filepath.Clean(configRoot)))
	return base + separator + hex.EncodeToString(sum[:4])
}

func sameProfile(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
