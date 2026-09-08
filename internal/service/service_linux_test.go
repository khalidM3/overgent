package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These run only on Linux. Everything that can be checked without a systemd
// user manager is in render_test.go and environment_test.go instead, so a macOS
// developer still gates the rendering and the classification; what is left here
// genuinely needs the platform.

func TestUnitIsScopedToTheProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	defaultRoot := filepath.Join(home, ".config", "Overgent")

	// The default profile must keep the original unit name, or upgrading
	// orphans a unit that is already installed and running.
	base := Manager{Executable: "/usr/local/bin/overgent", ConfigRoot: defaultRoot, Home: home, UID: 1000}
	if got := base.unitName(); got != unitBaseName+".service" {
		t.Fatalf("default profile unit = %q", got)
	}
	if want := filepath.Join(home, ".config", "systemd", "user", "overgent.service"); base.unitPath() != want {
		t.Fatalf("default unit path = %q, want %q", base.unitPath(), want)
	}

	// An isolated development profile must own a different unit and a different
	// file, otherwise it overwrites the production install.
	dev := base
	dev.ConfigRoot = filepath.Join(home, "dev-profile")
	if dev.unitName() == base.unitName() {
		t.Fatalf("development profile shares the production unit %q", dev.unitName())
	}
	if dev.unitPath() == base.unitPath() {
		t.Fatalf("development profile shares the production unit file %q", dev.unitPath())
	}
	again := dev
	again.ConfigRoot += "/"
	if again.unitName() != dev.unitName() {
		t.Fatalf("unit name is not stable across equivalent paths: %q vs %q", again.unitName(), dev.unitName())
	}
}

// systemd reads user units from $XDG_CONFIG_HOME/systemd/user when that is set,
// so writing to ~/.config unconditionally would put the unit somewhere systemd
// never looks.
func TestUnitPathHonoursXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	elsewhere := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", elsewhere)
	m := Manager{Executable: "/usr/local/bin/overgent", ConfigRoot: filepath.Join(elsewhere, "Overgent"), Home: home, UID: 1000}
	if want := filepath.Join(elsewhere, "systemd", "user", "overgent.service"); m.unitPath() != want {
		t.Fatalf("unit path = %q, want %q", m.unitPath(), want)
	}
	// The default profile follows XDG too, or an isolated one would be scoped
	// as if it were a development profile.
	if m.label() != unitBaseName {
		t.Fatalf("XDG default profile was scoped as a development profile: %q", m.label())
	}
}

func TestUnitRequiresExplicitSafePaths(t *testing.T) {
	for _, manager := range []Manager{
		{},
		{Executable: "relative", ConfigRoot: "/tmp/c", Home: "/tmp", UID: 1},
		{Executable: "/tmp/x\n", ConfigRoot: "/tmp/c", Home: "/tmp", UID: 1},
		{Executable: "/tmp/x", ConfigRoot: "/tmp/c\"", Home: "/tmp", UID: 1},
		{Executable: "/tmp/x", ConfigRoot: "/tmp/c", Home: "/tmp", UID: 0},
	} {
		if err := manager.validate(); err == nil {
			t.Fatalf("accepted invalid manager: %#v", manager)
		}
	}
}

func TestFailedInstallLeavesNoUnitBehind(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	m := Manager{
		Executable: "/usr/local/bin/overgent",
		ConfigRoot: filepath.Join(home, "profile"),
		Home:       home,
		UID:        os.Getuid(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := m.Install(ctx)
	if err == nil {
		// A real user manager accepted it. Undo rather than leaving a unit
		// installed on the machine that ran the tests.
		t.Cleanup(func() { _ = m.Remove(context.Background()) })
		t.Skip("systemd accepted the install in this environment")
	}
	// A unit file left behind reads as "installed" to every later check, so
	// callers conclude a service exists and nothing ever starts one.
	if _, statErr := os.Stat(m.unitPath()); !os.IsNotExist(statErr) {
		t.Fatalf("failed install left %s behind", m.unitPath())
	}
}

func TestInstallKeepsAUnitItDidNotCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	m := Manager{Executable: "/usr/local/bin/overgent", ConfigRoot: filepath.Join(home, "profile"), Home: home, UID: os.Getuid()}
	if err := os.MkdirAll(filepath.Dir(m.unitPath()), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(m.unitPath(), []byte("# earlier install\n"), 0o600); err != nil {
		t.Fatalf("seed unit: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := m.Install(ctx); err == nil {
		t.Cleanup(func() { _ = m.Remove(context.Background()) })
		t.Skip("systemd accepted the install in this environment")
	}
	// An earlier install may still be working; this call must not delete it.
	if _, err := os.Stat(m.unitPath()); err != nil {
		t.Fatalf("install removed a unit it did not create: %v", err)
	}
}

// Stop and Remove must tolerate a unit that is already gone, so an uninstall
// on a half-installed profile does not fail.
func TestStopAndRemoveTolerateAMissingUnit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	m := Manager{Executable: "/usr/local/bin/overgent", ConfigRoot: filepath.Join(home, "profile"), Home: home, UID: os.Getuid()}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for name, err := range map[string]error{"stop": m.Stop(ctx), "remove": m.Remove(ctx)} {
		// Without a user manager at all these report the environment problem,
		// which is a different and honest answer; anything else is a bug.
		if err != nil && !strings.Contains(err.Error(), "user manager") && !strings.Contains(err.Error(), "systemd") {
			t.Fatalf("%s of a missing unit failed: %v", name, err)
		}
	}
}
