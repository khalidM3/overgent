//go:build darwin

package service

import (
	"strings"
	"testing"
)

func TestLaunchAgentPlistUsesArgumentArrayAndRecovery(t *testing.T) {
	m := Manager{Executable: "/Applications/Stickguy.app/Contents/MacOS/stickguy", ConfigRoot: "/Users/test/Library/Application Support/Stickguy", Home: "/Users/test", UID: 501}
	body, err := m.plist()
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, expected := range []string{"<string>--config-root</string>", "<string>/Users/test/Library/Application Support/Stickguy</string>", "<key>KeepAlive</key>", "<true></true>", "<key>ThrottleInterval</key>"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("plist missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "sh -c") {
		t.Fatal("plist shell-concatenates arguments")
	}
}

func TestLaunchAgentRequiresExplicitSafePaths(t *testing.T) {
	for _, manager := range []Manager{{}, {Executable: "relative", ConfigRoot: "/tmp/c", Home: "/tmp", UID: 1}, {Executable: "/tmp/x\n", ConfigRoot: "/tmp/c", Home: "/tmp", UID: 1}} {
		if err := manager.validate(); err == nil {
			t.Fatalf("accepted invalid manager: %#v", manager)
		}
	}
}
