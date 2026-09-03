package main

import "testing"

func TestPlatformKey(t *testing.T) {
	for name, want := range map[string]string{"overgent_0.1.0_darwin_arm64.tar.gz": "darwin_arm64", "overgent_0.1.0_windows_amd64.zip": "windows_amd64"} {
		if got, ok := platformKey(name); !ok || got != want {
			t.Fatalf("platformKey(%q)=%q,%v", name, got, ok)
		}
	}
	if _, ok := platformKey("checksums.txt"); ok {
		t.Fatal("checksum file treated as an asset")
	}
}
