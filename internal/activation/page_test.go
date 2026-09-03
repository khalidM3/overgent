package activation

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestActivationPagePostsTicketOutsideURLAndEscapesValues(t *testing.T) {
	var page bytes.Buffer
	if err := writePage(&page, "https://api.overgent.com/v1/dashboard-activations", `synthetic-ticket-<unsafe>-value`); err != nil {
		t.Fatal(err)
	}
	html := page.String()
	if !strings.Contains(html, `method="post"`) || !strings.Contains(html, `type="hidden"`) || strings.Contains(html, `<unsafe>`) {
		t.Fatalf("unsafe activation page: %s", html)
	}
	if strings.Contains(html, `dashboard-activations?`) {
		t.Fatal("ticket moved into activation URL")
	}
}

func TestActivationSupportsLoopbackDashboardProxyPrefix(t *testing.T) {
	_, action, err := activationAction("http://127.0.0.1:5173/api")
	if err != nil {
		t.Fatal(err)
	}
	if action != "http://127.0.0.1:5173/api/v1/dashboard-activations" {
		t.Fatalf("action=%q", action)
	}
}

func TestActivationPageSubmitsItselfAndKeepsAButtonForNoScript(t *testing.T) {
	var page bytes.Buffer
	if err := writePage(&page, "https://api.overgent.com/v1/dashboard-activations", "synthetic-ticket-value-000"); err != nil {
		t.Fatal(err)
	}
	html := page.String()
	// The page is a stop on the way to somewhere else. Waiting for a press left
	// an unlabelled tab among the member's others while the app looked stalled.
	if !strings.Contains(html, activationScript) {
		t.Fatalf("activation page does not submit itself: %s", html)
	}
	// The press still has to be possible when the script cannot run.
	if !strings.Contains(html, `type="submit"`) {
		t.Fatalf("activation page has no no-script fallback: %s", html)
	}
}

func TestActivationScriptHashPinsTheContentSecurityPolicy(t *testing.T) {
	sum := sha256.Sum256([]byte(activationScript))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if scriptHash != want {
		t.Fatalf("scriptHash=%q want %q", scriptHash, want)
	}
	// 'unsafe-inline' would let any injected script run; the hash lets exactly
	// one run, and editing the script without the hash following breaks it shut.
	if strings.Contains(scriptHash, "unsafe-inline") {
		t.Fatal("activation CSP must not relax to unsafe-inline")
	}
}
