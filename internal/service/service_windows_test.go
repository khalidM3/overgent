package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These run only on Windows. Everything that can be checked without Task
// Scheduler - the XML, the argument quoting, the error classification - is in
// render_test.go, which runs everywhere.

func TestTaskIsScopedToTheProfile(t *testing.T) {
	home := `C:\Users\example`
	appData := filepath.Join(home, "AppData", "Roaming")
	t.Setenv("AppData", appData)
	defaultRoot := filepath.Join(appData, "Overgent")

	// The default profile must keep the original task name, or upgrading
	// orphans a task that is already installed and running.
	base := Manager{Executable: `C:\Program Files\Overgent\overgent.exe`, ConfigRoot: defaultRoot, Home: home, User: "example"}
	if got := base.label(); got != taskBaseName {
		t.Fatalf("default profile task = %q, want %q", got, taskBaseName)
	}

	// An isolated development profile must own a different task.
	dev := base
	dev.ConfigRoot = filepath.Join(home, "dev-profile")
	if dev.label() == base.label() {
		t.Fatalf("development profile shares the production task %q", dev.label())
	}
	if !strings.HasPrefix(dev.label(), taskBaseName+"-") {
		t.Fatalf("scoped task name = %q, want a %q prefix", dev.label(), taskBaseName)
	}
	again := dev
	again.ConfigRoot += `\`
	if again.label() != dev.label() {
		t.Fatalf("task name is not stable across equivalent paths: %q vs %q", again.label(), dev.label())
	}

	// The scoped name still has to reach the definition that gets registered.
	document, err := renderTask(taskSpec{Label: dev.label(), User: dev.User, Executable: dev.Executable, ConfigRoot: dev.ConfigRoot})
	if err != nil {
		t.Fatalf("renderTask: %v", err)
	}
	if !strings.Contains(document, dev.label()) {
		t.Fatalf("task definition does not declare the scoped name:\n%s", document)
	}
}

func TestTaskRequiresExplicitSafePaths(t *testing.T) {
	for _, manager := range []Manager{
		{},
		{Executable: "relative.exe", ConfigRoot: `C:\c`, Home: `C:\h`, User: "example"},
		{Executable: "C:\\x.exe\n", ConfigRoot: `C:\c`, Home: `C:\h`, User: "example"},
		{Executable: `C:\x.exe`, ConfigRoot: "C:\\c\"", Home: `C:\h`, User: "example"},
		{Executable: `C:\x.exe`, ConfigRoot: `C:\c`, Home: `C:\h`, User: ""},
	} {
		if err := manager.validate(); err == nil {
			t.Fatalf("accepted invalid manager: %#v", manager)
		}
	}
}

// Stop and Remove must tolerate a task that is already gone, so an uninstall on
// a half-installed profile does not fail.
func TestStopAndRemoveTolerateAMissingTask(t *testing.T) {
	home := t.TempDir()
	m := Manager{
		Executable: `C:\Program Files\Overgent\overgent.exe`,
		ConfigRoot: filepath.Join(home, "profile-that-was-never-installed"),
		Home:       home,
		User:       "example",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("stop of a missing task failed: %v", err)
	}
	if err := m.Remove(ctx); err != nil {
		t.Fatalf("remove of a missing task failed: %v", err)
	}
	// Status must say "not installed" rather than failing.
	status, err := m.Status(ctx)
	if err != nil {
		t.Fatalf("status of a missing task failed: %v", err)
	}
	if status.Installed || status.Running {
		t.Fatalf("a task that was never installed reported %+v", status)
	}
}
