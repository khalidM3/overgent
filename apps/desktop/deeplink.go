package main

import (
	"net/url"
	"strings"
)

func desktopDeepLinkProject(raw string) (string, bool) {
	if raw == "" || len(raw) > 2048 {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, desktopURLScheme()) || !strings.EqualFold(parsed.Host, "project") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	projectID := strings.Trim(parsed.Path, "/")
	if !validDeepLinkProjectID(projectID) {
		return "", false
	}
	return projectID, true
}

func validDeepLinkProjectID(value string) bool {
	if !strings.HasPrefix(value, "prj_") || len(value) < 5 || len(value) > 84 {
		return false
	}
	for _, char := range value[4:] {
		if char < 'A' || char > 'Z' && char < 'a' || char > 'z' && char < '0' || char > '9' && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

// addProjectURL is the shell's own route for registering a repository. It is
// reached three ways - the deep link below, the menu bar, and a hosted page in
// this window navigating to the shell's origin - so it is written once.
const addProjectURL = "/?desktop=onboarding&add=project"

// desktopUserAgentName is appended to the webview's user agent string.
//
// It is the only way a page served from the hosted origin can tell it is being
// rendered inside this window: the live Project view is hosted, so the native
// bridge it would otherwise ask is unreachable from it. `isDesktopShell` in
// apps/dashboard/src/native.ts matches " OvergentDesktop/", so the leading
// space WebKit inserts and the trailing slash are both part of the contract.
// It is a hint about what to say, never a capability: nothing is granted to a
// page because of what its user agent claims.
const desktopUserAgentName = "OvergentDesktop/1.0"

// desktopDeepLinkTarget maps an incoming scheme URL onto a window route.
//
// Only routes this application owns are accepted, and the result is always one
// of a fixed set of literals. A deep link is attacker-reachable — any page or
// document can ask the system to open one — so nothing from the URL is ever
// interpolated into the destination, which keeps a crafted link from steering
// the webview somewhere unintended.
func desktopDeepLinkTarget(raw string) (string, bool) {
	if raw == "" || len(raw) > 2048 {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, desktopURLScheme()) {
		return "", false
	}
	// A scheme URL puts the first path segment in Host ("overgent://new-project")
	// or in Path ("overgent:///new-project"), depending on how it was written.
	route := parsed.Host
	if route == "" {
		route = strings.TrimPrefix(parsed.Path, "/")
	}
	switch strings.ToLower(strings.Trim(route, "/")) {
	case "new-project":
		return addProjectURL, true
	case "", "open":
		return desktopStartURL(), true
	default:
		return "", false
	}
}
