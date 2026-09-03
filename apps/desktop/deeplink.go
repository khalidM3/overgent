package main

import (
	"net/url"
	"strings"
)

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
		return "/?desktop=onboarding&add=project", true
	case "", "open":
		return desktopStartURL(), true
	default:
		return "", false
	}
}
