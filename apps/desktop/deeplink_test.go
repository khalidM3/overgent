package main

import (
	"strings"
	"testing"
)

func TestDeepLinkTargetResolvesOnlyOwnRoutes(t *testing.T) {
	scheme := desktopURLScheme()
	const addProject = "/?desktop=onboarding&add=project"
	for _, accepted := range []struct{ raw, want string }{
		{scheme + "://new-project", addProject},
		{scheme + ":///new-project", addProject},
		{scheme + "://NEW-PROJECT/", addProject},
		{scheme + "://open", desktopStartURL()},
		{scheme + "://", desktopStartURL()},
	} {
		target, ok := desktopDeepLinkTarget(accepted.raw)
		if !ok || target != accepted.want {
			t.Fatalf("%q gave (%q, %v), want %q", accepted.raw, target, ok, accepted.want)
		}
	}
}

func TestDeepLinkTargetRefusesForeignSchemesAndUnknownRoutes(t *testing.T) {
	scheme := desktopURLScheme()
	for _, refused := range []string{
		"",
		"https://evil.example/new-project",
		"javascript:alert(1)",
		"file:///etc/passwd",
		"stickguy-other://new-project",
		scheme + "://unknown-route",
		scheme + "://new-project" + strings.Repeat("x", 4096),
	} {
		if target, ok := desktopDeepLinkTarget(refused); ok {
			t.Fatalf("%q was accepted as %q", refused, target)
		}
	}
}

func TestDeepLinkTargetNeverCarriesUrlContentIntoTheDestination(t *testing.T) {
	// A deep link is attacker-reachable: any page can ask the system to open
	// one. Query, fragment, and traversal in the incoming URL must not change
	// where the webview goes, so each of these resolves to the same literal.
	scheme := desktopURLScheme()
	const addProject = "/?desktop=onboarding&add=project"
	for _, raw := range []string{
		scheme + "://new-project?next=https://evil.example",
		scheme + "://new-project#/../../admin",
		scheme + "://new-project/../../admin",
		scheme + "://new-project?add=project&desktop=../evil",
	} {
		target, ok := desktopDeepLinkTarget(raw)
		if !ok || target != addProject {
			t.Fatalf("%q gave (%q, %v), want the fixed %q", raw, target, ok, addProject)
		}
	}
}
