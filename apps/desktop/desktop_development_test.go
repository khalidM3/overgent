//go:build darwin && !production

package main

import "testing"

func TestDevelopmentOriginsAreLoopbackOnly(t *testing.T) {
	if got := desktopStartURL(); got != "/?desktop=onboarding" {
		t.Fatalf("start URL=%q", got)
	}
	t.Setenv("STICKGUY_API_ORIGIN", "https://example.com")
	if got := desktopAPIBaseURL(); got != "http://127.0.0.1:3211" {
		t.Fatalf("external API accepted: %q", got)
	}
	t.Setenv("STICKGUY_API_ORIGIN", "http://127.0.0.1:4173")
	if got := desktopAPIBaseURL(); got != "http://127.0.0.1:4173" {
		t.Fatalf("loopback API rejected: %q", got)
	}
}
