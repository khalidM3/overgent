//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/khalidM3/overgent/internal/localbackend"
)

func TestBundledBackendArtifactsRequireBothFiles(t *testing.T) {
	// A build with only half the artifacts must report "no bundled backend"
	// rather than recording a path the service will later fail to start.
	root := t.TempDir()
	resources := filepath.Join(root, "Contents", "Resources", "backend")
	if err := os.MkdirAll(resources, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resources, "convex-local-backend"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := localbackend.Install(t.TempDir(), filepath.Join(resources, "convex-local-backend"), filepath.Join(resources, "backend-push.json")); err == nil {
		t.Fatal("install accepted a missing deploy payload")
	}
}
