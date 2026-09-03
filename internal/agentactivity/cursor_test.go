package agentactivity

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func cursorInput(t *testing.T, fields map[string]any) *strings.Reader {
	t.Helper()
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReader(string(encoded))
}

func TestCursorReadNamesTheFileAndJoinsOnConversationID(t *testing.T) {
	root := t.TempDir()
	event, err := ParseCursor("beforeReadFile", cursorInput(t, map[string]any{
		"conversation_id": "conv-42", "generation_id": "gen-7", "hook_event_name": "beforeReadFile",
		"workspace_roots": []string{root}, "file_path": filepath.Join(root, "backend", "refresh.go"),
		"content": "package backend\n\nfunc Refresh(userID string) error { return nil }\n",
	}), "")
	if err != nil {
		t.Fatal(err)
	}
	if event.Vendor != "cursor" || event.Kind != "PreToolUse" || event.Tool != "read" {
		t.Fatalf("unexpected lifecycle projection: %+v", event)
	}
	if !ReadTool(event.Tool) {
		t.Fatal("a Cursor file read must count as a read-set observation")
	}
	normalized, err := NormalizePaths(event, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.CandidatePaths) != 1 || normalized.CandidatePaths[0] != "backend/refresh.go" {
		t.Fatalf("read path did not survive normalization: %v", normalized.CandidatePaths)
	}
	// The join key is conversation_id, not session_id: a hook that omits
	// session_id must land on the same workstream as one that carries it.
	expected, alias, _ := WorkstreamIDFor("cursor", "conv-42")
	if event.WorkstreamID != expected || event.SessionAlias != alias {
		t.Fatalf("identity is not derived from conversation_id: %s %s", event.WorkstreamID, event.SessionAlias)
	}
	if !strings.HasPrefix(alias, "cursor-") || len(alias) != 13 {
		t.Fatalf("alias %q does not match the wire shape", alias)
	}
}

// The whole file arrives on stdin for a read. It must never reach the event, and
// a file larger than MaxInputBytes must still yield its read evidence — silently
// dropping it would leave a session reporting `observed` coverage with an empty
// read set, which is the exact failure ReadCoverage exists to prevent.
func TestCursorFileContentNeverReachesTheEventAndLargeFilesStillReport(t *testing.T) {
	root := t.TempDir()
	huge := strings.Repeat("SECRET_TOKEN=abcdefghijklmnop\n", (MaxInputBytes/30)+1000)
	if len(huge) <= MaxInputBytes {
		t.Fatalf("fixture is not larger than the ordinary hook bound: %d", len(huge))
	}
	event, err := ParseCursor("beforeReadFile", cursorInput(t, map[string]any{
		"conversation_id": "conv-42", "workspace_roots": []string{root},
		"file_path": filepath.Join(root, "big.go"), "content": huge,
	}), "")
	if err != nil {
		t.Fatalf("a large file must not drop the read observation: %v", err)
	}
	if len(event.CandidatePaths) != 1 {
		t.Fatalf("read evidence was dropped: %v", event.CandidatePaths)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"SECRET_TOKEN", "abcdefghijklmnop"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("file content reached the event: %s", forbidden)
		}
	}
}

func TestCursorPromptBecomesAClassifiedTitleAndNeverRawText(t *testing.T) {
	root := t.TempDir()
	event, err := ParseCursor("beforeSubmitPrompt", cursorInput(t, map[string]any{
		"conversation_id": "conv-42", "workspace_roots": []string{root},
		"prompt": "Implement the session view against Refresh",
	}), "")
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != "UserPromptSubmit" || event.SessionTitle != "Implement the session view against Refresh" {
		t.Fatalf("prompt did not become a bounded title: %+v", event)
	}

	// A prompt carrying a credential is rejected outright rather than redacted,
	// and a prompt past the 160-character bound yields no title rather than a
	// truncation presented as the session's name.
	//
	// Note the shape of the raw-output case. ClassifyCoordinationTitle collapses
	// whitespace before matching, so its line-anchored `stdout:`/`stderr:`
	// pattern only catches those markers at the very start of a title. The
	// unanchored markers still fire, which is what is asserted here. This is
	// pre-existing ADR-042 behaviour shared with the Claude title path, not
	// something Cursor changes; it is called out so the gap is not rediscovered
	// as a Cursor-specific one.
	for name, prompt := range map[string]string{
		"credential":  "use api_key=sk-live-0123456789abcdef to call the service",
		"environment": "run with DATABASE_URL=postgres://user:pw@host/db",
		"raw output":  "summarize this tool_result and continue",
		"oversize":    strings.Repeat("refactor the session boundary ", 20),
	} {
		event, err = ParseCursor("beforeSubmitPrompt", cursorInput(t, map[string]any{
			"conversation_id": "conv-42", "workspace_roots": []string{root}, "prompt": prompt,
		}), "")
		if err != nil {
			t.Fatalf("%s: observation must still succeed: %v", name, err)
		}
		if event.SessionTitle != "" {
			t.Fatalf("%s: prohibited or oversize prompt became a title: %q", name, event.SessionTitle)
		}
	}
}

// afterFileEdit and beforeSubmitPrompt carry no workspace root at all, so the
// session-scoped variable published by sessionStart is the only thing that can
// attribute them to a repository.
func TestCursorEventsWithoutARootFallBackToTheSessionVariable(t *testing.T) {
	root := t.TempDir()
	event, err := ParseCursor("afterFileEdit", cursorInput(t, map[string]any{
		"conversation_id": "conv-42", "file_path": filepath.Join(root, "frontend", "session.ts"),
	}), root)
	if err != nil {
		t.Fatal(err)
	}
	if event.CWD != root || event.Kind != "PostToolUse" || event.Tool != "edit" {
		t.Fatalf("unexpected projection without a reported root: %+v", event)
	}
	if _, err = ParseCursor("afterFileEdit", cursorInput(t, map[string]any{
		"conversation_id": "conv-42", "file_path": "frontend/session.ts",
	}), ""); err == nil {
		t.Fatal("an event with no root at all must be dropped rather than guessed")
	}
}

func TestCursorMultiRootWorkspaceCarriesEveryCandidate(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	event, err := ParseCursor("sessionStart", cursorInput(t, map[string]any{
		"conversation_id": "conv-42", "session_id": "sess-1",
		"workspace_roots": []string{first, second, first},
	}), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(event.CandidateRoots) != 2 || event.CandidateRoots[0] != first || event.CandidateRoots[1] != second {
		t.Fatalf("multi-root workspace was not carried for selection: %v", event.CandidateRoots)
	}
}

func TestCursorFailsClosedOnUnknownMalformedAndMismatchedInput(t *testing.T) {
	root := t.TempDir()
	if _, err := ParseCursor("beforeShellExecution", cursorInput(t, map[string]any{
		"conversation_id": "conv-42", "workspace_roots": []string{root},
	}), ""); err == nil {
		t.Fatal("an event Overgent does not install must not be assigned a guessed lifecycle")
	}
	if _, err := ParseCursor("stop", strings.NewReader("{not json"), root); err == nil {
		t.Fatal("malformed JSON must fail closed")
	}
	if _, err := ParseCursor("stop", cursorInput(t, map[string]any{
		"workspace_roots": []string{root},
	}), ""); err == nil {
		t.Fatal("an event with no conversation_id has no identity and must be rejected")
	}
	// The payload naming a different event than the hook it was installed under
	// means configuration and client have diverged. That is drift, not an event.
	if _, err := ParseCursor("stop", cursorInput(t, map[string]any{
		"conversation_id": "conv-42", "hook_event_name": "sessionStart", "workspace_roots": []string{root},
	}), ""); err == nil {
		t.Fatal("a mismatched hook_event_name must fail closed")
	}
}

func TestCursorProtectedPathsRejectTheWholeObservation(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{".env", "deploy/id_rsa", "config/secrets/token.pem"} {
		event, err := ParseCursor("beforeReadFile", cursorInput(t, map[string]any{
			"conversation_id": "conv-42", "workspace_roots": []string{root},
			"file_path": filepath.Join(root, path), "content": "irrelevant",
		}), "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = NormalizePaths(event, root); err == nil {
			t.Fatalf("%s must reject the whole observation", path)
		}
	}
}

func TestCursorLifecycleCoversExactlyTheInstalledEvents(t *testing.T) {
	for _, event := range CursorEvents {
		if !SupportedCursorEvent(event) {
			t.Fatalf("%s is installed but cannot be parsed", event)
		}
	}
	if len(cursorEvents) != len(CursorEvents) {
		t.Fatalf("parser understands %d events but %d are installed", len(cursorEvents), len(CursorEvents))
	}
}

// Cursor names each file it reads before reading it, which is the same class of
// evidence Claude provides and stronger than Codex's command classification.
func TestCursorReadCoverageIsObserved(t *testing.T) {
	if got := ReadCoverage("cursor", false); got != CoverageObserved {
		t.Fatalf("cursor read coverage is %q, want %q", got, CoverageObserved)
	}
}
