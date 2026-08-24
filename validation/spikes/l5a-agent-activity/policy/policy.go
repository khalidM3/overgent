package policy

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

type Profile uint8

const (
	Coordination Profile = iota
	Activity
	Conversation
)

type Candidate struct {
	ProjectID           string
	WorkspaceID         string
	Kind                string
	ObservedAt          time.Time
	Text                string
	ToolName            string
	Status              string
	Paths               []string
	CommandCategory     string
	VerificationOutcome string
	HasTranscriptPath   bool
	HasSystemPrompt     bool
	HasReasoning        bool
	HasSourceOrDiff     bool
	HasToolResult       bool
	HasRawCommand       bool
	HasRawOutput        bool
}

type Consent struct {
	Enabled       bool
	Paused        bool
	ProjectID     string
	OwnerMaximum  Profile
	MemberProfile Profile
}

type Event struct {
	ProjectID           string    `json:"projectId"`
	WorkspaceID         string    `json:"workspaceId"`
	Kind                string    `json:"kind"`
	ObservedAt          time.Time `json:"observedAt"`
	Profile             string    `json:"profile"`
	Text                string    `json:"text,omitempty"`
	ToolName            string    `json:"toolName,omitempty"`
	Status              string    `json:"status,omitempty"`
	Paths               []string  `json:"paths,omitempty"`
	CommandCategory     string    `json:"commandCategory,omitempty"`
	VerificationOutcome string    `json:"verificationOutcome,omitempty"`
}

var (
	errDisabled          = errors.New("activity sharing disabled")
	errPaused            = errors.New("activity sharing paused")
	errProjectMismatch   = errors.New("candidate project does not match consent")
	errProhibitedContent = errors.New("candidate contains prohibited content")
	errProfile           = errors.New("candidate exceeds selected sharing profile")
	errUnknownKind       = errors.New("unknown candidate kind")
	identifierPattern    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._:-]{0,63}$`)
)

var kindProfile = map[string]Profile{
	"session.status":         Activity,
	"turn.status":            Activity,
	"subagent.status":        Activity,
	"plan.visible":           Activity,
	"tool.activity":          Activity,
	"permission.required":    Activity,
	"path.affected":          Activity,
	"verification.outcome":   Activity,
	"conversation.user":      Conversation,
	"conversation.assistant": Conversation,
}

var protectedSegments = map[string]struct{}{
	".env": {}, ".ssh": {}, ".aws": {}, ".azure": {}, ".config/gcloud": {},
	".kube": {}, ".npmrc": {}, ".pypirc": {}, ".git-credentials": {},
	"id_rsa": {}, "id_ed25519": {}, "credentials": {}, "secrets": {},
}

var secretMarkers = []string{
	"authorization: bearer ", "bearer ", "begin private key", "begin rsa private key",
	"ghp_", "github_pat_", "sk-proj-", "akia", "xoxb-", "xoxp-", "password=",
}

func Project(candidate Candidate, consent Consent) (Event, error) {
	if !consent.Enabled {
		return Event{}, errDisabled
	}
	if consent.Paused {
		return Event{}, errPaused
	}
	if candidate.ProjectID == "" || candidate.ProjectID != consent.ProjectID {
		return Event{}, errProjectMismatch
	}
	required, ok := kindProfile[candidate.Kind]
	if !ok {
		return Event{}, errUnknownKind
	}
	effective := min(consent.OwnerMaximum, consent.MemberProfile)
	if required > effective {
		return Event{}, errProfile
	}
	if hasProhibitedFlags(candidate) || prohibitedText(candidate.Text) {
		return Event{}, errProhibitedContent
	}
	if err := validateKindFields(candidate); err != nil {
		return Event{}, err
	}
	if candidate.WorkspaceID == "" || candidate.ObservedAt.IsZero() {
		return Event{}, errors.New("candidate identity/time missing")
	}
	for _, candidatePath := range candidate.Paths {
		if err := validatePath(candidatePath); err != nil {
			return Event{}, fmt.Errorf("path rejected: %w", err)
		}
	}
	if candidate.ToolName != "" && !identifierPattern.MatchString(candidate.ToolName) {
		return Event{}, errors.New("tool name invalid")
	}
	if candidate.Status != "" && !slices.Contains([]string{"started", "running", "waiting", "passed", "failed", "completed", "blocked", "idle"}, candidate.Status) {
		return Event{}, errors.New("status invalid")
	}
	if candidate.CommandCategory != "" && !slices.Contains([]string{"build", "test", "lint", "format", "package", "git-read", "other"}, candidate.CommandCategory) {
		return Event{}, errors.New("command category invalid")
	}
	if candidate.VerificationOutcome != "" && !slices.Contains([]string{"not_run", "running", "passed", "failed", "unknown"}, candidate.VerificationOutcome) {
		return Event{}, errors.New("verification outcome invalid")
	}

	return Event{
		ProjectID:           candidate.ProjectID,
		WorkspaceID:         candidate.WorkspaceID,
		Kind:                candidate.Kind,
		ObservedAt:          candidate.ObservedAt.UTC(),
		Profile:             effective.String(),
		Text:                candidate.Text,
		ToolName:            candidate.ToolName,
		Status:              candidate.Status,
		Paths:               slices.Clone(candidate.Paths),
		CommandCategory:     candidate.CommandCategory,
		VerificationOutcome: candidate.VerificationOutcome,
	}, nil
}

func (profile Profile) String() string {
	switch profile {
	case Coordination:
		return "coordination"
	case Activity:
		return "activity"
	case Conversation:
		return "conversation"
	default:
		return "unknown"
	}
}

func hasProhibitedFlags(candidate Candidate) bool {
	return candidate.HasTranscriptPath || candidate.HasSystemPrompt || candidate.HasReasoning ||
		candidate.HasSourceOrDiff || candidate.HasToolResult || candidate.HasRawCommand || candidate.HasRawOutput
}

func validateKindFields(candidate Candidate) error {
	hasText := candidate.Text != ""
	hasTool := candidate.ToolName != ""
	hasStatus := candidate.Status != ""
	hasPaths := len(candidate.Paths) != 0
	hasCommand := candidate.CommandCategory != ""
	hasVerification := candidate.VerificationOutcome != ""

	allowed := false
	switch candidate.Kind {
	case "session.status", "turn.status", "subagent.status":
		allowed = hasStatus && !hasText && !hasTool && !hasPaths && !hasCommand && !hasVerification
	case "plan.visible":
		allowed = hasText && !hasTool && !hasStatus && !hasPaths && !hasCommand && !hasVerification
	case "tool.activity":
		allowed = hasTool && hasStatus && !hasText && !hasVerification
	case "permission.required":
		allowed = hasTool && hasStatus && !hasText && !hasCommand && !hasVerification
	case "path.affected":
		allowed = hasPaths && !hasText && !hasTool && !hasStatus && !hasCommand && !hasVerification
	case "verification.outcome":
		allowed = hasVerification && !hasText && !hasTool && !hasStatus && !hasPaths && !hasCommand
	case "conversation.user", "conversation.assistant":
		allowed = hasText && !hasTool && !hasStatus && !hasPaths && !hasCommand && !hasVerification
	}
	if !allowed {
		return errors.New("candidate fields do not match event allowlist")
	}
	return nil
}

func prohibitedText(text string) bool {
	if text == "" {
		return false
	}
	if len(text) > 2_000 || strings.ContainsRune(text, '\x00') || strings.Contains(text, "```") ||
		strings.Contains(text, "diff --git") || strings.Contains(text, "@@ -") {
		return true
	}
	lower := strings.ToLower(text)
	for _, marker := range secretMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func validatePath(candidatePath string) error {
	if candidatePath == "" || strings.ContainsRune(candidatePath, '\x00') || strings.Contains(candidatePath, `\`) || path.IsAbs(candidatePath) {
		return errors.New("path must be a non-empty normalized repository-relative path")
	}
	clean := path.Clean(candidatePath)
	if clean != candidatePath || clean == ".." || strings.HasPrefix(clean, "../") {
		return errors.New("path escapes or is not normalized")
	}
	lower := strings.ToLower(clean)
	if strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") {
		return errProhibitedContent
	}
	segments := strings.Split(lower, "/")
	for i, segment := range segments {
		if segment == ".env" || strings.HasPrefix(segment, ".env.") {
			return errProhibitedContent
		}
		if _, ok := protectedSegments[segment]; ok {
			return errProhibitedContent
		}
		if i+1 < len(segments) {
			if _, ok := protectedSegments[segment+"/"+segments[i+1]]; ok {
				return errProhibitedContent
			}
		}
	}
	return nil
}
