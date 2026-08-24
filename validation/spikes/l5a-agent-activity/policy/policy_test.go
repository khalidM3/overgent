package policy

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

var fixtureTime = time.Date(2026, 8, 23, 20, 0, 0, 0, time.UTC)

func baseCandidate(kind string) Candidate {
	return Candidate{ProjectID: "prj_fixture", WorkspaceID: "wsp_fixture", Kind: kind, ObservedAt: fixtureTime}
}

func consent(profile Profile) Consent {
	return Consent{Enabled: true, ProjectID: "prj_fixture", OwnerMaximum: profile, MemberProfile: profile}
}

func TestConsentAndNarrowerProfile(t *testing.T) {
	candidate := baseCandidate("conversation.user")
	candidate.Text = "Investigate the checkout state machine."

	if _, err := Project(candidate, Consent{}); !errors.Is(err, errDisabled) {
		t.Fatalf("disabled error = %v", err)
	}
	narrow := consent(Conversation)
	narrow.MemberProfile = Activity
	if _, err := Project(candidate, narrow); !errors.Is(err, errProfile) {
		t.Fatalf("narrow profile error = %v", err)
	}
	narrow.MemberProfile = Conversation
	narrow.Paused = true
	if _, err := Project(candidate, narrow); !errors.Is(err, errPaused) {
		t.Fatalf("paused error = %v", err)
	}
}

func TestActivityProjectionContainsOnlyAllowlistedFields(t *testing.T) {
	candidate := baseCandidate("tool.activity")
	candidate.ToolName = "Bash"
	candidate.Status = "completed"
	candidate.Paths = []string{"apps/dashboard/src/main.tsx"}
	candidate.CommandCategory = "test"

	event, err := Project(candidate, consent(Activity))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"rawcommand", "rawoutput", "toolresult", "transcript", "reasoning", "diffcontent", "sourcecontent"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("projected JSON contains forbidden field marker %q: %s", forbidden, encoded)
		}
	}
}

func TestEventKindCannotSmuggleUnexpectedFields(t *testing.T) {
	candidate := baseCandidate("tool.activity")
	candidate.ToolName = "Bash"
	candidate.Status = "completed"
	candidate.Text = "unexpected payload"
	if _, err := Project(candidate, consent(Activity)); err == nil || !strings.Contains(err.Error(), "event allowlist") {
		t.Fatalf("unexpected-field error = %v", err)
	}
}

func TestConversationAllowsBoundedVisibleText(t *testing.T) {
	candidate := baseCandidate("conversation.assistant")
	candidate.Text = "I am checking the dashboard authorization boundary."
	event, err := Project(candidate, consent(Conversation))
	if err != nil {
		t.Fatal(err)
	}
	if event.Text != candidate.Text || event.Profile != "conversation" {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestAlwaysProhibitedVendorFields(t *testing.T) {
	tests := map[string]func(*Candidate){
		"transcript":  func(candidate *Candidate) { candidate.HasTranscriptPath = true },
		"system":      func(candidate *Candidate) { candidate.HasSystemPrompt = true },
		"reasoning":   func(candidate *Candidate) { candidate.HasReasoning = true },
		"source":      func(candidate *Candidate) { candidate.HasSourceOrDiff = true },
		"tool result": func(candidate *Candidate) { candidate.HasToolResult = true },
		"command":     func(candidate *Candidate) { candidate.HasRawCommand = true },
		"output":      func(candidate *Candidate) { candidate.HasRawOutput = true },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := baseCandidate("conversation.assistant")
			candidate.Text = "Safe visible status."
			mutate(&candidate)
			if _, err := Project(candidate, consent(Conversation)); !errors.Is(err, errProhibitedContent) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestProtectedPathsAndSecretsRejectWholeEvent(t *testing.T) {
	paths := []string{".env", ".env.local", "config/.env.production", ".ssh/id_ed25519", ".aws/credentials", ".config/gcloud/application_default_credentials.json", "certs/client.pem", "../escape"}
	for _, candidatePath := range paths {
		t.Run(candidatePath, func(t *testing.T) {
			candidate := baseCandidate("path.affected")
			candidate.Paths = []string{"safe/file.go", candidatePath}
			if _, err := Project(candidate, consent(Activity)); err == nil {
				t.Fatal("protected path accepted")
			}
		})
	}
	texts := []string{"Authorization: Bearer fixture", "sk-proj-fixture", "```go\npackage secret\n```", "diff --git a/a b/a", strings.Repeat("x", 2_001)}
	for _, text := range texts {
		candidate := baseCandidate("conversation.user")
		candidate.Text = text
		if _, err := Project(candidate, consent(Conversation)); !errors.Is(err, errProhibitedContent) {
			t.Fatalf("prohibited text error = %v", err)
		}
	}
}

func TestProjectIsolationAndUnknownKindsFailClosed(t *testing.T) {
	candidate := baseCandidate("session.status")
	candidate.Status = "running"
	candidate.ProjectID = "prj_other"
	if _, err := Project(candidate, consent(Activity)); !errors.Is(err, errProjectMismatch) {
		t.Fatalf("project mismatch error = %v", err)
	}
	candidate.ProjectID = "prj_fixture"
	candidate.Kind = "vendor.new_sensitive_event"
	if _, err := Project(candidate, consent(Conversation)); !errors.Is(err, errUnknownKind) {
		t.Fatalf("unknown kind error = %v", err)
	}
}
