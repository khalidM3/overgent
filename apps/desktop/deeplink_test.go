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
		"overgent-other://new-project",
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

func TestProjectDeepLinkAcceptsOnlyAnOpaqueProjectID(t *testing.T) {
	projectID, ok := desktopDeepLinkProject(desktopURLScheme() + "://project/prj_atlas-01")
	if !ok || projectID != "prj_atlas-01" {
		t.Fatalf("project deep link = (%q, %v)", projectID, ok)
	}
	for _, raw := range []string{
		desktopURLScheme() + "://project/../../admin",
		desktopURLScheme() + "://project/prj_ok?next=https://evil.example",
		desktopURLScheme() + "://project/not-a-project",
		"https://overgent.com/project/prj_atlas",
	} {
		if projectID, ok := desktopDeepLinkProject(raw); ok {
			t.Fatalf("unsafe deep link %q accepted as %q", raw, projectID)
		}
	}
}

// The dashboard matches " OvergentDesktop/" in navigator.userAgent to decide
// whether a hosted page is inside the desktop window. Both halves of that
// contract are easy to change independently and impossible to notice breaking,
// because the failure is only wrong copy on one screen.
func TestDesktopUserAgentNameMatchesDashboardProbe(t *testing.T) {
	if !strings.HasPrefix(desktopUserAgentName, "OvergentDesktop/") {
		t.Fatalf("user agent name %q must start with OvergentDesktop/", desktopUserAgentName)
	}
	if strings.ContainsAny(desktopUserAgentName, " \t\r\n") {
		t.Fatalf("user agent name %q must not contain whitespace", desktopUserAgentName)
	}
}

// A second launch hands over its whole argument list, and only the URL in it is
// acted on. argv[0] is always a path, so the loop must not treat a filename
// that happens to contain the scheme as a link.
func TestDeepLinkFromArgumentsAcceptsOnlyAValidatedSchemeURL(t *testing.T) {
	scheme := desktopURLScheme()
	for _, testCase := range []struct {
		name  string
		args  []string
		want  string
		found bool
	}{
		{name: "no link", args: []string{"/opt/overgent/overgent-desktop"}},
		{name: "route", args: []string{"overgent-desktop", scheme + "://new-project"}, want: scheme + "://new-project", found: true},
		{name: "project", args: []string{"overgent-desktop", scheme + "://project/prj_abc123"}, want: scheme + "://project/prj_abc123", found: true},
		{name: "foreign scheme", args: []string{"overgent-desktop", "file:///etc/passwd"}},
		{name: "another application's scheme", args: []string{"overgent-desktop", "vscode://anthropic.claude-code/open"}},
		{name: "unknown route", args: []string{"overgent-desktop", scheme + "://settings"}},
		{name: "executable path is not a link", args: []string{"/home/x/overgent/overgent-desktop"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, ok := deepLinkFromArguments(testCase.args)
			if ok != testCase.found || got != testCase.want {
				t.Fatalf("deepLinkFromArguments(%v) = (%q,%v), want (%q,%v)", testCase.args, got, ok, testCase.want, testCase.found)
			}
		})
	}
}
