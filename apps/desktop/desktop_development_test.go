//go:build darwin && !production

package main

import "testing"

func TestDevelopmentOriginsAllowLoopbackAndHTTPSOnly(t *testing.T) {
	if got := desktopStartURL(); got != "/?desktop=onboarding" {
		t.Fatalf("start URL=%q", got)
	}
	t.Setenv("OVERGENT_API_ORIGIN", "https://example.com")
	if got := desktopAPIBaseURL(); got != "https://example.com" {
		t.Fatalf("HTTPS shared API rejected: %q", got)
	}
	t.Setenv("OVERGENT_API_ORIGIN", "http://127.0.0.1:4173")
	if got := desktopAPIBaseURL(); got != "http://127.0.0.1:4173" {
		t.Fatalf("loopback API rejected: %q", got)
	}
	t.Setenv("OVERGENT_API_ORIGIN", "http://example.com")
	if got := desktopAPIBaseURL(); got != "http://127.0.0.1:3211" {
		t.Fatalf("insecure remote API accepted: %q", got)
	}
	t.Setenv("OVERGENT_API_ORIGIN", "http://user:secret@127.0.0.1:3211")
	if got := desktopAPIBaseURL(); got != "http://127.0.0.1:3211" {
		t.Fatalf("credentialed origin accepted: %q", got)
	}
	t.Setenv("OVERGENT_API_ORIGIN", "https://example.com/api/")
	if got := desktopAPIBaseURL(); got != "https://example.com/api" {
		t.Fatalf("shared HTTPS API path rejected: %q", got)
	}
}

// A dashboard ticket is only redeemable on the backend that minted it, so the
// development activation origin has to follow the Project's backend rather than
// always answering with Vite.
func TestDevelopmentActivationOriginFollowsTheProjectBackend(t *testing.T) {
	t.Setenv("OVERGENT_DASHBOARD_ORIGIN", "http://127.0.0.1:5173/api")
	for _, testCase := range []struct{ name, backend, want string }{
		{"development backend uses the dev server", "http://127.0.0.1:3211", "http://127.0.0.1:5173/api"},
		{"other loopback backend uses the dev server", "http://localhost:4211", "http://127.0.0.1:5173/api"},
		{"unset backend falls back to the dev server", "", "http://127.0.0.1:5173/api"},
		{"remote backend serves its own dashboard", "https://api.example.com", "https://api.example.com"},
		{"remote backend keeps no trailing slash", "https://api.example.com/", "https://api.example.com"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := desktopTeamActivationOrigin(testCase.backend); got != testCase.want {
				t.Fatalf("activation origin for %q = %q, want %q", testCase.backend, got, testCase.want)
			}
		})
	}
}
