//go:build darwin || linux || windows

package main

import "testing"

// A URL that arrives before the window exists must still be shown, because a
// second instance can hand one over at any point during startup.
func TestDeepLinkRouterHoldsAURLUntilThereIsAWindow(t *testing.T) {
	router := &deepLinkRouter{}
	router.deliver(desktopURLScheme() + "://new-project")
	router.deliver(desktopURLScheme() + "://project/prj_abc123")
	var seen []string
	router.attach(func(raw string) { seen = append(seen, raw) })
	if len(seen) != 1 || seen[0] != desktopURLScheme()+"://project/prj_abc123" {
		t.Fatalf("replayed %v, want only the most recent URL", seen)
	}
	router.deliver(desktopURLScheme() + "://new-project")
	if len(seen) != 2 || seen[1] != desktopURLScheme()+"://new-project" {
		t.Fatalf("later delivery not routed: %v", seen)
	}
}
