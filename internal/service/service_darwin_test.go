//go:build darwin

package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLaunchAgentPlistUsesArgumentArrayAndRecovery(t *testing.T) {
	m := Manager{Executable: "/Applications/Overgent.app/Contents/MacOS/overgent", ConfigRoot: "/Users/test/Library/Application Support/Overgent", Home: "/Users/test", UID: 501}
	body, err := m.plist()
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, expected := range []string{"<string>--config-root</string>", "<string>/Users/test/Library/Application Support/Overgent</string>", "<key>KeepAlive</key>", "<true/>", "<key>ThrottleInterval</key>"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("plist missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "sh -c") {
		t.Fatal("plist shell-concatenates arguments")
	}
	// launchd rejects the expanded empty element with a bare "Bootstrap failed:
	// 5: Input/output error", so every install fails while plutil -lint still
	// reports the file as valid. Only the self-closing form bootstraps.
	for _, rejected := range []string{"<true></true>", "<false></false>"} {
		if strings.Contains(text, rejected) {
			t.Fatalf("plist uses %q, which launchd refuses to bootstrap:\n%s", rejected, text)
		}
	}
}

func TestLaunchAgentRequiresExplicitSafePaths(t *testing.T) {
	for _, manager := range []Manager{{}, {Executable: "relative", ConfigRoot: "/tmp/c", Home: "/tmp", UID: 1}, {Executable: "/tmp/x\n", ConfigRoot: "/tmp/c", Home: "/tmp", UID: 1}} {
		if err := manager.validate(); err == nil {
			t.Fatalf("accepted invalid manager: %#v", manager)
		}
	}
}

func TestLabelIsScopedToTheProfile(t *testing.T) {
	home := "/Users/example"
	defaultRoot := filepath.Join(home, "Library", "Application Support", "Overgent")

	// The default profile must keep the original label, or upgrading orphans a
	// LaunchAgent that is already installed and running.
	base := Manager{Executable: "/usr/local/bin/overgent", ConfigRoot: defaultRoot, Home: home, UID: 501}
	if got := base.label(); got != defaultLabel {
		t.Fatalf("default profile label = %q, want %q", got, defaultLabel)
	}
	if !strings.HasSuffix(base.plistPath(), defaultLabel+".plist") {
		t.Fatalf("default plist path = %q", base.plistPath())
	}

	// An isolated development profile must own a different job and a different
	// plist, otherwise it overwrites the production install and only one of the
	// two can ever be bootstrapped.
	dev := base
	dev.ConfigRoot = "/Users/example/dev-profile"
	if dev.label() == base.label() {
		t.Fatalf("development profile shares the production label %q", dev.label())
	}
	if dev.plistPath() == base.plistPath() {
		t.Fatalf("development profile shares the production plist %q", dev.plistPath())
	}
	if !strings.HasPrefix(dev.label(), defaultLabel+".") {
		t.Fatalf("scoped label = %q, want a %q prefix", dev.label(), defaultLabel)
	}

	// The same profile must always resolve to the same job.
	again := dev
	again.ConfigRoot = "/Users/example/dev-profile/"
	if again.label() != dev.label() {
		t.Fatalf("label is not stable across equivalent paths: %q vs %q", again.label(), dev.label())
	}

	// A scoped label still has to name the right job inside the plist.
	body, err := dev.plist()
	if err != nil {
		t.Fatalf("plist: %v", err)
	}
	if !strings.Contains(string(body), dev.label()) {
		t.Fatalf("plist does not declare the scoped label:\n%s", body)
	}
}

func TestAlreadyLoadedRecognisesLaunchdConflicts(t *testing.T) {
	for _, text := range []string{"Bootstrap failed: 37: Operation already in progress", "Service already loaded", "Load failed: 17: File exists"} {
		if !alreadyLoaded(&commandError{output: text}) {
			t.Fatalf("alreadyLoaded(%q) = false, want true", text)
		}
	}
	// An unloading job is not the same as a loaded one, and must keep retrying.
	if alreadyLoaded(&commandError{output: "Bootstrap failed: 5: Input/output error"}) {
		t.Fatal("EIO must not be treated as an already-loaded job")
	}
}

func TestFailedInstallLeavesNoPlistBehind(t *testing.T) {
	home := t.TempDir()
	m := Manager{
		// launchctl cannot bootstrap a job for a home that is not this user's,
		// so Install fails after writing the plist - the case that matters.
		Executable: "/usr/local/bin/overgent",
		ConfigRoot: filepath.Join(home, "profile"),
		Home:       home,
		UID:        999999,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := m.Install(ctx); err == nil {
		t.Skip("launchctl accepted the bootstrap in this environment")
	}
	// A plist left behind reads as "installed" to every later check, so callers
	// conclude a service exists and nothing ever starts one.
	if _, err := os.Stat(m.plistPath()); !os.IsNotExist(err) {
		t.Fatalf("failed install left %s behind", m.plistPath())
	}
}

func TestInstallKeepsAPlistItDidNotCreate(t *testing.T) {
	home := t.TempDir()
	m := Manager{Executable: "/usr/local/bin/overgent", ConfigRoot: filepath.Join(home, "profile"), Home: home, UID: 999999}
	if err := os.MkdirAll(filepath.Dir(m.plistPath()), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(m.plistPath(), []byte("<plist>earlier install</plist>"), 0o600); err != nil {
		t.Fatalf("seed plist: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := m.Install(ctx); err == nil {
		t.Skip("launchctl accepted the bootstrap in this environment")
	}
	// An earlier install may still be working; this call must not delete it.
	if _, err := os.Stat(m.plistPath()); err != nil {
		t.Fatalf("install removed a plist it did not create: %v", err)
	}
}
