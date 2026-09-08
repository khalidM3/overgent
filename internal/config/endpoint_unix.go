//go:build !windows

package config

import "path/filepath"

// endpointFor names the local IPC endpoint for a profile.
//
// On unix that is a Unix socket inside the profile root, where the 0700
// directory and the 0600 socket are what restrict it to the owning user.
func endpointFor(root string) string {
	return filepath.Join(root, "service.sock")
}
