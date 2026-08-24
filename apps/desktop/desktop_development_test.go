//go:build darwin && !production

package main

import "testing"

func TestDevelopmentURLIsLoopbackOnly(t *testing.T) {
	t.Setenv("STICKGUY_DESKTOP_DEV_URL", "")
	if got := desktopStartURL(); got != "http://127.0.0.1:5173/?desktop=preview" { t.Fatalf("default URL=%q", got) }
	t.Setenv("STICKGUY_DESKTOP_DEV_URL", "https://example.com/?live=1")
	if got := desktopStartURL(); got != "/?desktop=preview" { t.Fatalf("external URL accepted: %q", got) }
	t.Setenv("STICKGUY_DESKTOP_DEV_URL", "http://127.0.0.1:4173/?desktop=preview")
	if got := desktopStartURL(); got != "http://127.0.0.1:4173/?desktop=preview" { t.Fatalf("loopback URL rejected: %q", got) }
}
