package sessiontranscript

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write builds an isolated synthetic transcript; tests never read real
// contributor sessions.
func write(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadsVisibleTextThinkingAndToolNames(t *testing.T) {
	path := write(t,
		`{"type":"ai-title","sessionId":"s1","aiTitle":"Generated title"}`,
		`{"type":"custom-title","sessionId":"s1","customTitle":"Rotate session boundary"}`,
		`{"type":"user","sessionId":"s1","gitBranch":"feature/rotation","timestamp":"2026-08-25T10:00:00Z","message":{"role":"user","content":"Explain the rotation boundary."}}`,
		`{"type":"assistant","sessionId":"s1","timestamp":"2026-08-25T10:00:05Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"I should read the session module first.","signature":"sig"},{"type":"text","text":"I'll start with the session module."},{"type":"tool_use","name":"Read","input":{"file_path":"/etc/passwd"}}]}}`,
	)
	session, err := Read(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if session.Title != "Rotate session boundary" {
		t.Fatalf("a member-set title must win: %q", session.Title)
	}
	if session.Branch != "feature/rotation" || session.SessionID != "s1" {
		t.Fatalf("branch=%q session=%q", session.Branch, session.SessionID)
	}
	kinds := make([]string, 0, len(session.Messages))
	for _, message := range session.Messages {
		kinds = append(kinds, message.Kind)
	}
	want := []string{KindUser, KindThinking, KindAssistant, KindTool}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("kinds=%v want=%v", kinds, want)
	}
	if session.Messages[1].Text != "I should read the session module first." {
		t.Fatalf("vendor-recorded thinking must be preserved: %q", session.Messages[1].Text)
	}
	// A tool becomes a name only. Its input is never content.
	if session.Messages[3].Tool != "Read" || session.Messages[3].Text != "" {
		t.Fatalf("tool message leaked input: %#v", session.Messages[3])
	}
}

func TestRawToolResultsAndOutputNeverBecomeContent(t *testing.T) {
	path := write(t,
		`{"type":"user","sessionId":"s1","toolUseResult":{"stdout":"SECRET_TOKEN=abc123"},"message":{"role":"user","content":[{"type":"tool_result","content":"SECRET_TOKEN=abc123"}]}}`,
		`{"type":"assistant","sessionId":"s1","message":{"role":"assistant","content":[{"type":"tool_result","content":"drwxr-xr-x  home"}]}}`,
		`{"type":"attachment","sessionId":"s1","attachment":{"content":"ignored"}}`,
		`{"type":"assistant","sessionId":"s1","message":{"role":"assistant","content":[{"type":"text","text":"Done."}]}}`,
	)
	session, err := Read(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Messages) != 1 || session.Messages[0].Text != "Done." {
		t.Fatalf("only visible assistant text may survive: %#v", session.Messages)
	}
}

func TestMalformedLinesAreSkippedAndTailIsBounded(t *testing.T) {
	lines := []string{`{"type":"assistant","sessionId":"s1"`, `not json at all`, `{"type":"assistant","sessionId":"s1","message":{"role":"assistant","content":[{"type":"text","text":"survived"}]}}`}
	session, err := Read(write(t, lines...), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Messages) != 1 || session.Messages[0].Text != "survived" {
		t.Fatalf("one bad record must not hide a session: %#v", session.Messages)
	}

	// A very long session is read from the tail and keeps only the newest turns.
	long := make([]string, 0, 800)
	for index := 0; index < 800; index++ {
		long = append(long, fmt.Sprintf(`{"type":"assistant","sessionId":"s1","message":{"role":"assistant","content":[{"type":"text","text":"message %d"}]}}`, index))
	}
	bounded, err := Read(write(t, long...), 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(bounded.Messages) != 50 || bounded.Messages[49].Text != "message 799" {
		t.Fatalf("expected the newest 50 messages, got %d ending %q", len(bounded.Messages), bounded.Messages[len(bounded.Messages)-1].Text)
	}
}

func TestRejectsPathsThatAreNotTranscripts(t *testing.T) {
	for _, path := range []string{"", "relative/session.jsonl", "/tmp/not-a-transcript.txt", "/tmp/missing-file.jsonl"} {
		if _, err := Read(path, 10); err == nil {
			t.Fatalf("expected %q to be rejected", path)
		}
	}
}
