package sessiontranscript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// codexRecords mirrors the real rollout envelope shape; tests never read real
// contributor sessions.
func codexRecords(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

func TestCodexReadsTheVisibleConversationNotInjectedContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-2026-08-25T10-00-00-019f54c4-fd53-7d71-a61b-9b552fc3f730.jsonl")
	body := codexRecords(
		`{"timestamp":"2026-08-25T10:00:00Z","type":"session_meta","payload":{"id":"019f54c4-fd53-7d71-a61b-9b552fc3f730","session_id":"019f54c4-fd53-7d71-a61b-9b552fc3f730","cwd":"/repo"}}`,
		`{"timestamp":"2026-08-25T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"Rotate the session boundary","images":[]}}`,
		`{"timestamp":"2026-08-25T10:00:02Z","type":"event_msg","payload":{"type":"agent_reasoning","text":"Check the session module first."}}`,
		`{"timestamp":"2026-08-25T10:00:03Z","type":"response_item","payload":{"type":"custom_tool_call","name":"exec","input":"cat /etc/passwd","call_id":"c1"}}`,
		`{"timestamp":"2026-08-25T10:00:04Z","type":"response_item","payload":{"type":"custom_tool_call_output","output":"root:x:0:0 SECRET_TOKEN=abc","call_id":"c1"}}`,
		`{"timestamp":"2026-08-25T10:00:05Z","type":"event_msg","payload":{"type":"agent_message","message":"Rotation now happens on permission change.","phase":"commentary"}}`,
		// Raw model I/O duplicates the turn and adds injected context; it must
		// not be read as conversation.
		`{"timestamp":"2026-08-25T10:00:06Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"[80] tool exec call: injected framing"}]}}`,
		`{"timestamp":"2026-08-25T10:00:07Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Rotation now happens on permission change."}]}}`,
		// Encrypted reasoning is vendor-held and must never be read.
		`{"timestamp":"2026-08-25T10:00:08Z","type":"response_item","payload":{"type":"reasoning","summary":[],"encrypted_content":"ZW5jcnlwdGVkLXJlYXNvbmluZw=="}}`,
	)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := Read(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if session.Vendor != "codex" {
		t.Fatalf("vendor=%q", session.Vendor)
	}
	if session.SessionID != "019f54c4-fd53-7d71-a61b-9b552fc3f730" {
		t.Fatalf("sessionId=%q", session.SessionID)
	}
	// Codex records no title of its own; the opening request labels the session.
	if session.Title != "Rotate the session boundary" {
		t.Fatalf("title=%q", session.Title)
	}
	var kinds []string
	for _, message := range session.Messages {
		kinds = append(kinds, message.Kind)
	}
	want := []string{KindUser, KindThinking, KindTool, KindAssistant}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("kinds=%v want=%v", kinds, want)
	}
	joined := ""
	for _, message := range session.Messages {
		joined += message.Text + "\x00" + message.Tool + "\n"
	}
	for _, forbidden := range []string{"SECRET_TOKEN", "root:x:0:0", "cat /etc/passwd", "encrypted", "ZW5jcnlwdGVk", "injected framing"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("leaked %q into conversation: %s", forbidden, joined)
		}
	}
	// The turn appears once, from the event stream, not twice.
	assistants := 0
	for _, message := range session.Messages {
		if message.Kind == KindAssistant {
			assistants++
		}
	}
	if assistants != 1 {
		t.Fatalf("assistant turn duplicated: %d", assistants)
	}
}

func TestCodexSurfacesOperatingInstructionsAsSystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-x-019f54c4-fd53-7d71-a61b-9b552fc3f730.jsonl")
	body := codexRecords(
		`{"timestamp":"2026-08-25T10:00:00Z","type":"session_meta","payload":{"id":"019f54c4-fd53-7d71-a61b-9b552fc3f730"}}`,
		`{"timestamp":"2026-08-25T10:00:01Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<permissions instructions>\nFilesystem sandboxing is on."}]}}`,
		`{"timestamp":"2026-08-25T10:00:02Z","type":"event_msg","payload":{"type":"user_message","message":"Start the audit"}}`,
	)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := Read(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Messages) != 2 || session.Messages[0].Kind != KindSystem {
		t.Fatalf("messages=%#v", session.Messages)
	}
	if !strings.Contains(session.Messages[0].Text, "Filesystem sandboxing is on.") {
		t.Fatalf("system text=%q", session.Messages[0].Text)
	}
	// The instruction block is not a request, so it never becomes the title.
	if session.Title != "Start the audit" {
		t.Fatalf("title=%q", session.Title)
	}
}

func TestLocateCodexRolloutFindsBySessionIDAndRefusesUnsafeIDs(t *testing.T) {
	home := t.TempDir()
	day := filepath.Join(home, ".codex", "sessions", "2026", "08", "25")
	if err := os.MkdirAll(day, 0o700); err != nil {
		t.Fatal(err)
	}
	id := "019f54c4-fd53-7d71-a61b-9b552fc3f730"
	want := filepath.Join(day, "rollout-2026-08-25T10-00-00-"+id+".jsonl")
	for _, name := range []string{want, filepath.Join(day, "rollout-2026-08-25T09-00-00-019f0000-0000-0000-0000-000000000000.jsonl")} {
		if err := os.WriteFile(name, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := LocateCodexRollout(home, id); got != want {
		t.Fatalf("located %q want %q", got, want)
	}
	for _, unsafe := range []string{"", "../../etc/passwd", "019f54c4-fd53-7d71-a61b-9b552fc3f73", "*", "019f54c4_fd53_7d71_a61b_9b552fc3f730"} {
		if got := LocateCodexRollout(home, unsafe); got != "" {
			t.Fatalf("accepted unsafe session id %q -> %q", unsafe, got)
		}
	}
	if got := LocateCodexRollout(home, "019f0000-0000-0000-0000-00000000ffff"); got != "" {
		t.Fatal("unknown session must not resolve to a file")
	}
}

func TestOversizedRecordsAreSkippedAndTruncationIsLinear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-y-019f54c4-fd53-7d71-a61b-9b552fc3f730.jsonl")
	// An inline image record dwarfs any readable message and must be skipped.
	huge := `{"timestamp":"t","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"` + strings.Repeat("A", maxRecordBytes+16) + `"}]}}`
	// A long-but-readable instruction block is truncated, not dropped.
	long := `{"timestamp":"t","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"` + strings.Repeat("B", 40_000) + `"}]}}`
	body := codexRecords(huge, long, `{"timestamp":"t","type":"event_msg","payload":{"type":"user_message","message":"Go"}}`)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := Read(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Messages) != 2 {
		t.Fatalf("expected the skipped record to be absent: %d messages", len(session.Messages))
	}
	if got := len(session.Messages[0].Text); got > MaxMessageBytes {
		t.Fatalf("message not bounded: %d bytes", got)
	}
	if strings.Contains(session.Messages[0].Text, "A") {
		t.Fatal("oversized record leaked into the conversation")
	}
}

// The Codex desktop app reports turns as `item_completed` events rather than the
// CLI's `user_message`/`agent_message`, and is inconsistent about the case of its
// content part type. Reading only the CLI shape left every desktop session with no
// user turn, so no title was derived, no intent was published, and the session could
// never take part in semantic coordination.
func TestCodexReadsDesktopAppItemCompletedTurns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-2026-08-27T22-35-15-01a046dd-7d33-7f70-9815-ce617c25ad14.jsonl")
	body := codexRecords(
		`{"timestamp":"2026-08-27T22:35:15Z","type":"session_meta","payload":{"id":"01a046dd-7d33-7f70-9815-ce617c25ad14","session_id":"01a046dd-7d33-7f70-9815-ce617c25ad14","cwd":"/repo","originator":"Codex Desktop"}}`,
		// The desktop app injects its own framing as a developer turn.
		`{"timestamp":"2026-08-27T22:35:16Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"<app-context>desktop framing</app-context>"}]}}`,
		// The member's own request. Note the lowercase part type.
		`{"timestamp":"2026-08-27T22:35:17Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","id":"i1","content":[{"type":"text","text":"Invalidate all login sessions after a privilege change."}]}}}`,
		// The agent's reply. The desktop app capitalizes this part type.
		`{"timestamp":"2026-08-27T22:35:18Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"i2","content":[{"type":"Text","text":"I will add the invalidation path."}]}}}`,
		// Command output and file changes stay out of shared content entirely.
		`{"timestamp":"2026-08-27T22:35:19Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"i3","command":["/bin/sh","-c","cat .env"],"aggregated_output":"SECRET_TOKEN=abc"}}}`,
		`{"timestamp":"2026-08-27T22:35:20Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"FileChange","id":"i4","changes":[{"path":"/repo/backend/security.go"}]}}}`,
		// Reasoning items are vendor-held and are not conversation.
		`{"timestamp":"2026-08-27T22:35:21Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"Reasoning","id":"i5","content":[{"type":"text","text":"internal deliberation"}]}}}`,
	)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	session, err := Read(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if session.Title != "Invalidate all login sessions after a privilege change." {
		t.Fatalf("desktop session title = %q", session.Title)
	}
	var user, assistant int
	for _, message := range session.Messages {
		switch message.Kind {
		case KindUser:
			user++
		case KindAssistant:
			assistant++
			if message.Text != "I will add the invalidation path." {
				t.Fatalf("assistant text = %q", message.Text)
			}
		}
	}
	if user != 1 || assistant != 1 {
		t.Fatalf("desktop turns: user=%d assistant=%d", user, assistant)
	}
	for _, prohibited := range []string{"SECRET_TOKEN", "cat .env", "internal deliberation", "backend/security.go"} {
		for _, message := range session.Messages {
			if strings.Contains(message.Text, prohibited) {
				t.Fatalf("desktop transcript leaked %q", prohibited)
			}
		}
	}
}
