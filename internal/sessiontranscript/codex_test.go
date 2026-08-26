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
