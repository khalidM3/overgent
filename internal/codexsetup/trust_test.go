package codexsetup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khalidM3/overgent/internal/codexappserver"
)

func TestUntrustedHooksReportNeedsReviewRatherThanActive(t *testing.T) {
	project := t.TempDir()
	codexHome := t.TempDir()
	original := inspectTrust
	t.Cleanup(func() { inspectTrust = original })
	inspectTrust = func(_ Manager, _ context.Context, _, _ string, _ bool) TrustReport {
		return TrustReport{Method: TrustMethodManual, Total: 9, Trusted: 0,
			Pending: []string{"sessionStart"}, Guidance: ReviewGuidance}
	}
	manager := Manager{ProjectRoot: project, ConfigRoot: filepath.Join(t.TempDir(), "state"),
		Executable: "/usr/local/bin/overgent", CodexHome: codexHome}

	status, err := manager.Setup()
	if err != nil {
		t.Fatal(err)
	}
	// The files are installed, so the binding is configured — but Codex will
	// skip these hooks silently, and reporting "active" is exactly the lie that
	// made this failure invisible.
	if !status.Configured {
		t.Fatal("setup should still report a configured binding")
	}
	if status.Hooks != "needs_review" {
		t.Fatalf("untrusted hooks reported as %q", status.Hooks)
	}
	if status.Trust.Guidance == "" {
		t.Fatal("a member who must act was given no guidance")
	}
	if status.HookPath != filepath.Join(codexHome, "hooks.json") {
		t.Fatalf("hooks were not installed at the user layer: %q", status.HookPath)
	}
	if reported, statusErr := manager.Status(); statusErr != nil || reported.Hooks != "needs_review" {
		t.Fatalf("status=%#v err=%v", reported, statusErr)
	}
}

func TestRemoveKeepsSharedHooksUntilRemoveHooks(t *testing.T) {
	trustedForTest(t)
	codexHome := t.TempDir()
	configRoot := filepath.Join(t.TempDir(), "state")
	first := Manager{ProjectRoot: t.TempDir(), ConfigRoot: configRoot, Executable: "/usr/local/bin/overgent", CodexHome: codexHome}
	second := Manager{ProjectRoot: t.TempDir(), ConfigRoot: configRoot, Executable: "/usr/local/bin/overgent", CodexHome: codexHome}
	if _, err := first.Setup(); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Setup(); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(codexHome, "hooks.json")
	before, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.Remove(); err != nil {
		t.Fatal(err)
	}
	// Hooks are shared by every registered project. Removing one project must
	// not silently disarm observation for the others.
	after, err := os.ReadFile(hookPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("removing one project disturbed shared hooks: err=%v", err)
	}
	if status, statusErr := second.Status(); statusErr != nil || status.Hooks != "active" {
		t.Fatalf("second project lost its hooks: status=%#v err=%v", status, statusErr)
	}
	if err = second.RemoveHooks(); err != nil {
		t.Fatal(err)
	}
	remaining, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(remaining), "agent-hook --vendor codex") {
		t.Fatalf("managed hooks survived teardown: %s", remaining)
	}
	if err = second.RemoveHooks(); err != nil {
		t.Fatalf("teardown is not idempotent: %v", err)
	}
}

func TestAppendTrustTablesIsAppendOnlyAndSkipsExistingKeys(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	unrelated := "model = \"gpt-5.4-mini\"\n\n[projects.\"/tmp/x\"]\ntrust_level = \"trusted\"\n"
	if err := os.WriteFile(configPath, []byte(unrelated), 0o600); err != nil {
		t.Fatal(err)
	}
	edits := []codexappserver.TrustEdit{
		{Key: "/codex/hooks.json:session_start:0:0", Hash: "sha256:aaa"},
		{Key: "/codex/hooks.json:stop:0:0", Hash: "sha256:bbb"},
	}
	if err := appendTrustTables(configPath, edits); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	// Every pre-existing byte must survive verbatim: this fallback only ever
	// appends, because rewriting the member's Codex config risks losing it.
	if !strings.HasPrefix(string(written), unrelated) {
		t.Fatalf("existing configuration was rewritten: %q", written)
	}
	for _, want := range []string{`[hooks.state."/codex/hooks.json:session_start:0:0"]`, `trusted_hash = "sha256:aaa"`} {
		if !strings.Contains(string(written), want) {
			t.Fatalf("missing %q in %q", want, written)
		}
	}
	if err = appendTrustTables(configPath, edits); err != nil {
		t.Fatal(err)
	}
	repeated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	// A duplicate table header makes the whole file unparseable and would take
	// Codex down with it, so a second pass must change nothing.
	if string(repeated) != string(written) {
		t.Fatalf("second append duplicated tables: %q", repeated)
	}
	if permissions, statErr := os.Stat(configPath); statErr != nil || permissions.Mode().Perm() != 0o600 {
		t.Fatalf("append widened config permissions: %v %v", permissions.Mode().Perm(), statErr)
	}
}

func TestSelectManagedHooksIgnoresForeignAndManagedEntries(t *testing.T) {
	const ours = "'/bin/overgent' --config-root '/state' agent-hook --vendor codex"
	hooks := []codexappserver.Hook{
		{Key: "a", HandlerType: "command", Command: ours, SourcePath: "/codex/hooks.json"},
		// Another Overgent profile: same shape, different config root.
		{Key: "b", HandlerType: "command", Command: "'/bin/overgent' --config-root '/other' agent-hook --vendor codex", SourcePath: "/codex/hooks.json"},
		// A managed hook is trusted by policy and is never ours to rewrite.
		{Key: "c", HandlerType: "command", Command: ours, SourcePath: "/codex/hooks.json", IsManaged: true},
		// Someone else's unrelated hook.
		{Key: "d", HandlerType: "command", Command: "/usr/bin/true", SourcePath: "/codex/hooks.json"},
		// Our command, but resolved from a different file.
		{Key: "e", HandlerType: "command", Command: ours, SourcePath: "/elsewhere/hooks.json"},
	}
	selected := selectManagedHooks(hooks, "/codex/hooks.json", ours)
	if len(selected) != 1 || selected[0].Key != "a" {
		t.Fatalf("selection touched hooks Overgent does not own: %#v", selected)
	}
}

func TestTrustReportSatisfiedRequiresObservedHooks(t *testing.T) {
	// Zero observed hooks means Codex could not see the binding at all. That is
	// the silent failure this work exists to surface, never a success.
	if (TrustReport{Total: 0, Trusted: 0}).Satisfied() {
		t.Fatal("an empty trust report reported satisfaction")
	}
	if (TrustReport{Total: 9, Trusted: 8}).Satisfied() {
		t.Fatal("a partially trusted binding reported satisfaction")
	}
	if !(TrustReport{Total: 9, Trusted: 9}).Satisfied() {
		t.Fatal("a fully trusted binding was not satisfied")
	}
}
