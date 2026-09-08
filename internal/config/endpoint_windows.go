//go:build windows

package config

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// pipePrefix is the only namespace Windows offers for named pipes. A pipe is
// not a filesystem object, so unlike the unix socket it inherits no protection
// from the profile directory's mode: the daemon package is what restricts it to
// the owning user, by way of the security descriptor it creates the pipe with.
const pipePrefix = `\\.\pipe\overgent-`

// endpointFor names the local IPC endpoint for a profile.
//
// The name is derived from the profile root rather than fixed, for the same
// reason internal/service scopes its job label by config root: a development
// build using an isolated OVERGENT_CONFIG_ROOT and a production install must
// not claim the same endpoint, or only one of them can ever listen.
//
// A hash rather than the path itself, because the pipe namespace is flat and
// admits neither the separators nor the drive letters a Windows path contains.
func endpointFor(root string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(root))))
	return pipePrefix + hex.EncodeToString(sum[:8])
}
