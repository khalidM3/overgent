package agentactivity

import (
	"path/filepath"
	"testing"
)

func TestParseAndNormalizeClaudeEdit(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	event, err := Parse("claude", []byte(`{"session_id":"session-raw-secret","cwd":"`+root+`","hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":"`+filepath.Join(root, "src/nav.tsx")+`","old_string":"source is discarded"}}`))
	if err != nil {
		t.Fatal(err)
	}
	event, err = NormalizePaths(event, root)
	if err != nil {
		t.Fatal(err)
	}
	if event.WorkstreamID[:10] != "wrk_agent_" || event.SessionAlias == "session-raw-secret" || len(event.CandidatePaths) != 1 || event.CandidatePaths[0] != "src/nav.tsx" {
		t.Fatalf("event=%+v", event)
	}
}

func TestProtectedAndEscapingPathsRejectWholeEvent(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	for _, candidate := range []string{filepath.Join(root, ".env.local"), filepath.Join(root, "secrets", "token.txt"), filepath.Join(root, "..", "outside.txt")} {
		event, err := Parse("codex", []byte(`{"session_id":"s","cwd":"`+root+`","hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_input":{"command":"*** Update File: `+candidate+`\n+secret"}}`))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = NormalizePaths(event, root); err == nil {
			t.Fatalf("accepted protected path %s", candidate)
		}
	}
}

func TestUnknownEventAndOversizeFailClosed(t *testing.T) {
	if _, err := Parse("claude", []byte(`{"session_id":"s","cwd":"/tmp","hook_event_name":"FutureEvent"}`)); err == nil {
		t.Fatal("unknown event accepted")
	}
	if _, err := Parse("claude", make([]byte, MaxInputBytes+1)); err == nil {
		t.Fatal("oversize input accepted")
	}
}
