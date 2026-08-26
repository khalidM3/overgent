package agentactivity

import (
	"path/filepath"
	"strings"
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

func TestHookNamesTheTranscriptRatherThanCarryingContent(t *testing.T) {
	event, err := Parse("claude", []byte(`{"session_id":"s","cwd":"/tmp","hook_event_name":"UserPromptSubmit","prompt":"Explain the navigation architecture","transcript_path":"/tmp/session.jsonl"}`))
	if err != nil || event.TranscriptPath != "/tmp/session.jsonl" {
		t.Fatalf("event=%+v err=%v", event, err)
	}
}

func TestClassifyAllowsQuotedCodeButRejectsSecretsAndRawOutput(t *testing.T) {
	// ADR-036: an agent conversation is unreadable without quoted code, and the
	// member explicitly chose to share it.
	for _, text := range []string{
		"Explain the navigation architecture",
		"```ts\nconst source = true\n```",
		"diff --git a/nav.tsx b/nav.tsx\n+const x = 1",
		"Use the sessionRotation() helper in src/auth.ts",
	} {
		if _, err := ClassifyMessage(Message{Kind: "assistant", Text: text}); err != nil {
			t.Fatalf("rejected allowed message %q: %v", text, err)
		}
	}
	// Vendor-recorded reasoning is Project-shareable content.
	if _, err := ClassifyMessage(Message{Kind: "thinking", Text: "I should read the session module first."}); err != nil {
		t.Fatalf("thinking must be shareable: %v", err)
	}
	// ADR-038: naming a credential file is ordinary conversation. Only the
	// material itself rejects a message.
	for _, text := range []string{
		"read .env.local to see which variables are set",
		"check the .env file before running the migration",
		"I added the key to .env.production; restart the service",
		"Compare with MAX_RETRIES == 5 before changing it",
		"The stdout of that command looked fine to me",
	} {
		if _, err := ClassifyMessage(Message{Kind: "assistant", Text: text}); err != nil {
			t.Fatalf("rejected a harmless mention %q: %v", text, err)
		}
	}
	for _, text := range []string{
		"API_KEY=super-secret", "export DATABASE_URL=postgres://x",
		"Update this in .env.local: DATABASE_URL=postgres://user:pw@host/db",
		"Here is the file:\nSTRIPE_KEY=sk_live_abcdefghijklmno\nDB_PASS=hunter2",
		"Bearer abcdefghijklmnopqrstuvwxyz", "-----BEGIN RSA PRIVATE KEY-----", "password: hunter2hunter2",
		"tool_result: 42 rows", "stdout: total 12", "transcript_path /tmp/x.jsonl",
	} {
		if _, err := ClassifyMessage(Message{Kind: "user", Text: text}); err == nil {
			t.Fatalf("accepted prohibited message %q", text)
		}
	}
	// Tool names are activity metadata, never shareable conversation.
	if _, err := ClassifyMessage(Message{Kind: "tool", Text: "Read"}); err == nil {
		t.Fatal("tool messages must never be shareable content")
	}
}

func TestClassifyCoordinationTitleIsBoundedBeforeUpload(t *testing.T) {
	if got, err := ClassifyCoordinationTitle("  Rotate   browser sessions  "); err != nil || got != "Rotate browser sessions" {
		t.Fatalf("title=%q err=%v", got, err)
	}
	for _, value := range []string{"", "API_KEY=super-secret", "Bearer abcdefghijklmnopqrstuvwxyz", strings.Repeat("x", 161)} {
		if _, err := ClassifyCoordinationTitle(value); err == nil {
			t.Fatalf("accepted prohibited title %q", value)
		}
	}
}
