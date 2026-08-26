//go:build darwin && !production

package main

import "testing"

func TestDevelopmentOriginsAllowLoopbackAndHTTPSOnly(t *testing.T) {
	if got := desktopStartURL(); got != "/?desktop=onboarding" {
		t.Fatalf("start URL=%q", got)
	}
	t.Setenv("STICKGUY_API_ORIGIN", "https://example.com")
	if got := desktopAPIBaseURL(); got != "https://example.com" {
		t.Fatalf("HTTPS shared API rejected: %q", got)
	}
	t.Setenv("STICKGUY_API_ORIGIN", "http://127.0.0.1:4173")
	if got := desktopAPIBaseURL(); got != "http://127.0.0.1:4173" {
		t.Fatalf("loopback API rejected: %q", got)
	}
	t.Setenv("STICKGUY_API_ORIGIN", "http://example.com")
	if got := desktopAPIBaseURL(); got != "http://127.0.0.1:3211" {
		t.Fatalf("insecure remote API accepted: %q", got)
	}
	t.Setenv("STICKGUY_API_ORIGIN", "http://user:secret@127.0.0.1:3211")
	if got := desktopAPIBaseURL(); got != "http://127.0.0.1:3211" {
		t.Fatalf("credentialed origin accepted: %q", got)
	}
	t.Setenv("STICKGUY_API_ORIGIN", "https://example.com/api/")
	if got := desktopAPIBaseURL(); got != "https://example.com/api" {
		t.Fatalf("shared HTTPS API path rejected: %q", got)
	}
}
