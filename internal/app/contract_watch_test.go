//go:build darwin

package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
	gitobs "github.com/stickguy/stickguy/internal/git"
	"github.com/stickguy/stickguy/internal/store"
)

type contractWatchFixture struct {
	root      string
	db        *store.Store
	service   *Service
	workspace config.Workspace
}

func newContractWatchFixture(t *testing.T) contractWatchFixture {
	t.Helper()
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(root, "internal", "session"), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	workspace := config.Workspace{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", MemberID: "mem_fixture", SessionID: "ses_fixture", Root: root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque"}
	if err = db.UpsertWorkspace(ctx, store.Workspace{ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID, MemberID: workspace.MemberID, DeviceID: "dev_fixture", SessionID: workspace.SessionID, Root: root, Baseline: workspace.Baseline, Fingerprint: workspace.Fingerprint}); err != nil {
		t.Fatal(err)
	}
	service := &Service{store: db, cfg: config.Config{Version: 1, DeviceID: "dev_fixture", Workspaces: []config.Workspace{workspace}}}
	return contractWatchFixture{root: root, db: db, service: service, workspace: workspace}
}

func (f contractWatchFixture) write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.root, filepath.FromSlash(path)), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func payloadsOfKind(t *testing.T, db *store.Store, kind string) []map[string]any {
	t.Helper()
	queue, err := db.Pending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for _, event := range queue {
		if event.Kind != kind {
			continue
		}
		var envelope eventEnvelopeForTest
		if err := json.Unmarshal(event.Payload, &envelope); err != nil {
			t.Fatal(err)
		}
		out = append(out, envelope.Payload)
	}
	return out
}

const rotateBefore = `package session

import "context"

// Rotate turns the session key over.
func Rotate(ctx context.Context, key string) (string, error) { return key, nil }
`

const rotateBodyEdit = `package session

import "context"

// Rotate turns the session key over, now with a comment change.
func Rotate(ctx context.Context, key string) (string, error) {
	println(key)
	return key, nil
}
`

const rotateSignatureEdit = `package session

import "context"

func Rotate(ctx context.Context, key string, at int64) (string, error) { return key, nil }
`

func TestContractFingerprintsPublishOnlyWhenTheExportedSurfaceMoves(t *testing.T) {
	ctx := context.Background()
	fixture := newContractWatchFixture(t)
	fixture.write(t, "internal/session/rotate.go", rotateBefore)
	fixture.write(t, "README.md", "# not fingerprintable\n")
	entries := []gitobs.Entry{
		{Path: "internal/session/rotate.go", States: gitobs.States{Worktree: &gitobs.Change{Status: "modified"}}},
		{Path: "README.md", States: gitobs.States{Worktree: &gitobs.Change{Status: "modified"}}},
	}

	fixture.service.publishContractFingerprints(ctx, fixture.workspace, entries)
	published := payloadsOfKind(t, fixture.db, "workspace.contract_fingerprints_reported")
	if len(published) != 1 {
		t.Fatalf("first scan published %d fingerprint events, want 1", len(published))
	}
	files, _ := published[0]["entries"].([]any)
	if len(files) != 1 {
		t.Fatalf("entries=%v, want only the fingerprintable path", published[0]["entries"])
	}
	if published[0]["workstreamId"] != fixture.workspace.WorkstreamID {
		t.Fatalf("publishing workstream=%v, want %s", published[0]["workstreamId"], fixture.workspace.WorkstreamID)
	}
	first := files[0].(map[string]any)
	if first["path"] != "internal/session/rotate.go" {
		t.Fatalf("path=%v", first["path"])
	}
	symbols := first["symbols"].([]any)
	if len(symbols) != 1 || symbols[0].(map[string]any)["name"] != "Rotate" {
		t.Fatalf("symbols=%v", symbols)
	}
	originalHash, _ := first["fileContractHash"].(string)

	// A republication with no change at all must not add an event.
	fixture.service.publishContractFingerprints(ctx, fixture.workspace, entries)
	if published = payloadsOfKind(t, fixture.db, "workspace.contract_fingerprints_reported"); len(published) != 1 {
		t.Fatalf("an unchanged scan published %d events, want 1", len(published))
	}

	// A body-only edit leaves the contract hash alone, so nothing is published.
	fixture.write(t, "internal/session/rotate.go", rotateBodyEdit)
	fixture.service.publishContractFingerprints(ctx, fixture.workspace, entries)
	if published = payloadsOfKind(t, fixture.db, "workspace.contract_fingerprints_reported"); len(published) != 1 {
		t.Fatalf("a body-only edit published %d events, want 1", len(published))
	}

	// A signature change does publish, with the new hash.
	fixture.write(t, "internal/session/rotate.go", rotateSignatureEdit)
	fixture.service.publishContractFingerprints(ctx, fixture.workspace, entries)
	published = payloadsOfKind(t, fixture.db, "workspace.contract_fingerprints_reported")
	if len(published) != 2 {
		t.Fatalf("a signature change published %d events, want 2", len(published))
	}
	changed := published[1]["entries"].([]any)[0].(map[string]any)
	if changed["fileContractHash"] == originalHash {
		t.Fatal("a signature change republished the original contract hash")
	}
}

func TestFingerprintPayloadCarriesNoSourceOrAbsolutePath(t *testing.T) {
	ctx := context.Background()
	fixture := newContractWatchFixture(t)
	fixture.write(t, "internal/session/rotate.go", rotateBefore)
	fixture.service.publishContractFingerprints(ctx, fixture.workspace, []gitobs.Entry{
		{Path: "internal/session/rotate.go", States: gitobs.States{Worktree: &gitobs.Change{Status: "modified"}}},
	})
	queue, err := fixture.db.Pending(ctx)
	if err != nil || len(queue) == 0 {
		t.Fatalf("queue=%d err=%v", len(queue), err)
	}
	text := string(queue[len(queue)-1].Payload)
	for _, prohibited := range []string{fixture.root, "return key, nil", "turns the session key over", "package session"} {
		if strings.Contains(text, prohibited) {
			t.Fatalf("fingerprint event leaked %q: %s", prohibited, text)
		}
	}
	if !strings.Contains(text, "func Rotate(ctx context.Context, key string) (string, error)") {
		t.Fatalf("normalized signature missing: %s", text)
	}
}

func TestReadSetIsCapturedFromInspectionToolsAndDedupedPerSession(t *testing.T) {
	ctx := context.Background()
	fixture := newContractWatchFixture(t)
	fixture.write(t, "internal/session/rotate.go", rotateBefore)
	const session = "wrk_agent_0123456789abcdef0123456789abcdef"
	inspect := daemon.Request{
		Method: "agent_event", AgentVendor: "claude", AgentCWD: fixture.root,
		AgentWorkstreamID: session, AgentSessionAlias: "claude-a1b2c3", AgentEvent: "PreToolUse",
		AgentStatus: "active", AgentAction: "inspecting", AgentTool: "Read",
		AgentPaths: []string{filepath.Join(fixture.root, "internal", "session", "rotate.go")},
	}
	if response := fixture.service.handle(ctx, inspect); !response.OK {
		t.Fatalf("response=%#v", response)
	}
	published := payloadsOfKind(t, fixture.db, "session.read_set_reported")
	if len(published) != 1 {
		t.Fatalf("published %d read-set events, want 1", len(published))
	}
	if published[0]["sessionWorkstreamId"] != session {
		t.Fatalf("session=%v", published[0]["sessionWorkstreamId"])
	}
	entries := published[0]["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["path"] != "internal/session/rotate.go" {
		t.Fatalf("entries=%v", entries)
	}

	// Re-reading the same unchanged file is one entry per (session, path).
	if response := fixture.service.handle(ctx, inspect); !response.OK {
		t.Fatalf("response=%#v", response)
	}
	if published = payloadsOfKind(t, fixture.db, "session.read_set_reported"); len(published) != 1 {
		t.Fatalf("a repeated read published %d events, want 1", len(published))
	}

	// Re-reading after the contract moved records the new hash.
	fixture.write(t, "internal/session/rotate.go", rotateSignatureEdit)
	if response := fixture.service.handle(ctx, inspect); !response.OK {
		t.Fatalf("response=%#v", response)
	}
	if published = payloadsOfKind(t, fixture.db, "session.read_set_reported"); len(published) != 2 {
		t.Fatalf("a moved contract published %d events, want 2", len(published))
	}
	before := published[0]["entries"].([]any)[0].(map[string]any)["fileContractHashAtRead"]
	after := published[1]["entries"].([]any)[0].(map[string]any)["fileContractHashAtRead"]
	if before == after {
		t.Fatal("the second read recorded the original contract hash")
	}
}

func TestEditsAndUnfingerprintablePathsAreNotReadSetEntries(t *testing.T) {
	ctx := context.Background()
	fixture := newContractWatchFixture(t)
	fixture.write(t, "internal/session/rotate.go", rotateBefore)
	fixture.write(t, "README.md", "# prose\n")
	edit := daemon.Request{
		Method: "agent_event", AgentVendor: "claude", AgentCWD: fixture.root,
		AgentWorkstreamID: "wrk_agent_0123456789abcdef0123456789abcdef", AgentSessionAlias: "claude-a1b2c3",
		AgentEvent: "PreToolUse", AgentStatus: "active", AgentAction: "editing", AgentTool: "Edit",
		AgentPaths: []string{filepath.Join(fixture.root, "internal", "session", "rotate.go")},
	}
	if response := fixture.service.handle(ctx, edit); !response.OK {
		t.Fatalf("response=%#v", response)
	}
	inspectProse := edit
	inspectProse.AgentTool = "Read"
	inspectProse.AgentPaths = []string{filepath.Join(fixture.root, "README.md")}
	if response := fixture.service.handle(ctx, inspectProse); !response.OK {
		t.Fatalf("response=%#v", response)
	}
	if published := payloadsOfKind(t, fixture.db, "session.read_set_reported"); len(published) != 0 {
		t.Fatalf("published %d read-set events, want none", len(published))
	}
}

func TestBeginWorkAnticipatedPathsJoinTheReadSetAfterTheIntent(t *testing.T) {
	ctx := context.Background()
	fixture := newContractWatchFixture(t)
	fixture.write(t, "internal/session/rotate.go", rotateBefore)
	response := fixture.service.handle(ctx, daemon.Request{
		Method: "begin_work", WorkspaceID: fixture.workspace.ID, IdempotencyKey: "begin_1",
		Title: "Consume the rotation contract", IntendedOutcome: "Depend on Rotate",
		AnticipatedPaths: []string{"internal/session/rotate.go", "../outside/escape.go", "secrets/keys.ts"},
	})
	if !response.OK {
		t.Fatalf("response=%#v", response)
	}
	queue, err := fixture.db.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var intentSequence, readSetSequence int64
	for _, event := range queue {
		switch event.Kind {
		case "workstream.intent_reported":
			intentSequence = event.Sequence
		case "session.read_set_reported":
			readSetSequence = event.Sequence
		}
	}
	if intentSequence == 0 || readSetSequence == 0 || readSetSequence <= intentSequence {
		t.Fatalf("read set must follow the intent: intent=%d readSet=%d", intentSequence, readSetSequence)
	}
	published := payloadsOfKind(t, fixture.db, "session.read_set_reported")
	entries := published[0]["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["path"] != "internal/session/rotate.go" {
		t.Fatalf("escaping and protected paths were not dropped: %v", entries)
	}
	if published[0]["sessionWorkstreamId"] != fixture.workspace.WorkstreamID {
		t.Fatalf("session=%v", published[0]["sessionWorkstreamId"])
	}
}
