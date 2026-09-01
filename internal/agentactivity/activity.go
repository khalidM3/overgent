package agentactivity

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const MaxInputBytes = 256 << 10
const MaxMessageBytes = 8000

type Message struct {
	Kind string
	Text string
}

type Event struct {
	Vendor         string
	CWD            string
	WorkstreamID   string
	SessionAlias   string
	Kind           string
	Status         string
	Action         string
	Tool           string
	AgentType      string
	SubagentAlias  string
	CandidatePaths []string
	// CandidateRoots holds every workspace root a vendor reported for this
	// session, for vendors that report more than one. Cursor sends
	// `workspace_roots` as an array, and only the daemon knows which of them
	// this device has registered, so the choice is made there rather than here.
	CandidateRoots []string
	// SessionTitle is a vendor-visible title already passed through
	// ClassifyCoordinationTitle (ADR-042). It carries classifier output only;
	// the text it was derived from never reaches this struct.
	SessionTitle string
	// TranscriptPath is the vendor-named transcript for this session (ADR-036).
	TranscriptPath string
	// VendorSessionID is the raw id the vendor used. It is used locally to find
	// a session record that the hook does not name, and is never published.
	VendorSessionID string
}

var identifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._:-]{0,63}$`)

// Parse decodes a Claude- or Codex-shaped hook payload, which share a record
// format: top-level `session_id`, `cwd`, and `hook_event_name`.
//
// Cursor is deliberately not routed here. Its payload shares none of those three
// fields, and its beforeReadFile carries the whole file being read, which this
// function's map[string]any decode would materialize. Cursor has its own decoder
// in cursor.go; see ParseCursor.
func Parse(vendor string, input []byte) (Event, error) {
	if vendor != "codex" && vendor != "claude" {
		return Event{}, errors.New("unsupported activity vendor")
	}
	if len(input) == 0 || len(input) > MaxInputBytes {
		return Event{}, errors.New("activity hook input is empty or too large")
	}
	var raw map[string]any
	if err := json.Unmarshal(input, &raw); err != nil {
		return Event{}, errors.New("invalid activity hook JSON")
	}
	sessionID, _ := raw["session_id"].(string)
	cwd, _ := raw["cwd"].(string)
	kind, _ := raw["hook_event_name"].(string)
	if sessionID == "" || cwd == "" || kind == "" || len(sessionID) > 512 || len(cwd) > 4096 {
		return Event{}, errors.New("activity hook identity is missing or invalid")
	}
	canonicalCWD, err := filepath.Abs(cwd)
	if err != nil {
		return Event{}, errors.New("activity hook working directory is invalid")
	}
	workstreamID, sessionAlias, ok := WorkstreamIDFor(vendor, sessionID)
	if !ok {
		return Event{}, errors.New("activity hook identity is missing or invalid")
	}
	event := Event{
		Vendor: vendor, CWD: canonicalCWD,
		WorkstreamID:    workstreamID,
		VendorSessionID: sessionID,
		SessionAlias:    sessionAlias,
		Kind:            kind,
	}
	tool, _ := raw["tool_name"].(string)
	if tool != "" {
		if len(tool) > 64 || !identifier.MatchString(tool) {
			return Event{}, errors.New("activity tool name is invalid")
		}
		event.Tool = tool
	}
	agentType, _ := raw["agent_type"].(string)
	if agentType != "" {
		if len(agentType) > 64 || !identifier.MatchString(agentType) {
			return Event{}, errors.New("activity agent type is invalid")
		}
		event.AgentType = agentType
	}
	if agentID, _ := raw["agent_id"].(string); agentID != "" {
		agentSum := sha256.Sum256([]byte("stickguy.subagent.v1\x00" + vendor + "\x00" + sessionID + "\x00" + agentID))
		event.SubagentAlias = fmt.Sprintf("sub-%x", agentSum[:3])
	}

	switch kind {
	case "SessionStart":
		event.Status, event.Action = "active", "Session started"
	case "UserPromptSubmit":
		event.Status, event.Action = "active", "Working on a new request"
	case "PreToolUse":
		event.Status, event.Action = "active", toolAction(tool, false)
	case "PermissionRequest":
		event.Status, event.Action = "waiting", "Waiting for permission"
	case "PostToolUse":
		event.Status, event.Action = "active", toolAction(tool, true)
	case "PostToolUseFailure":
		event.Status, event.Action = "error", toolLabel(tool)+" failed"
	case "SubagentStart":
		event.Status, event.Action = "active", "Started "+agentLabel(agentType)
	case "SubagentStop":
		event.Status, event.Action = "active", "Finished "+agentLabel(agentType)
	case "Stop":
		event.Status, event.Action = "idle", "Turn finished"
	case "SessionEnd":
		event.Status, event.Action = "done", "Session ended"
	default:
		return Event{}, errors.New("unsupported activity hook event")
	}

	// Supported hooks do not carry assistant text or reasoning; they name the
	// vendor transcript instead. ADR-036 reads that file locally so the session
	// owner can see their own session.
	if path, ok := raw["transcript_path"].(string); ok && path != "" && len(path) <= 4096 {
		event.TranscriptPath = path
	}

	if toolInput, ok := raw["tool_input"].(map[string]any); ok {
		event.CandidatePaths = candidatePaths(tool, toolInput)
	}
	return event, nil
}

var (
	credentialPattern = regexp.MustCompile(`(?i)(bearer\s+[a-z0-9._~+/=-]{12,}|(?:api[_-]?key|access[_-]?token|client[_-]?secret|password|passwd|secret)\s*[:=]\s*\S+|\b(?:sk|ghp|github_pat|xox[baprs])[-_][a-z0-9_-]{12,}|\bAKIA[A-Z0-9]{16}\b)`)
	// An assignment discloses a value wherever it appears, not only at line
	// start. The trailing class excludes "==" so a comparison in quoted code is
	// not mistaken for a secret.
	environmentPattern = regexp.MustCompile(`\b[A-Z][A-Z0-9_]{2,}\s*=[^=\s]\S*`)
	privateKeyPattern  = regexp.MustCompile(`(?i)-----BEGIN [A-Z ]*PRIVATE KEY-----`)
	// Naming a file is not disclosing its contents, and agents discuss ".env"
	// constantly. What must never leave is the file's *content*, which is what
	// environmentPattern catches. A single mention no longer rejects a message.
	toolOutputPattern = regexp.MustCompile(`(?im)(\btool_result\b|\btranscript_path\b|^\s*(?:stdout|stderr)\s*:)`)
)

var shareableKinds = map[string]bool{"user": true, "assistant": true, "thinking": true, "system": true}

// WorkstreamIDFor derives the stable per-session workstream identity from a
// vendor session id. Activity hooks and the MCP server must agree on this
// derivation: a coordination brief is filtered by the workstream it is
// requested for, so an MCP client that computed a different identity than its
// own hooks could never be shown a finding routed to its session.
// The identity passed as sessionID is whatever the vendor uses to join its own
// hooks together: Claude and Codex send `session_id`, Cursor sends
// `conversation_id`. Cursor's `session_id` appears only on sessionStart and
// sessionEnd, so hashing it would give one chat a new workstream per hook.
//
// What this derives is the local session handle, not the identity that reaches
// the hosted service. The daemon scopes the handle to an enrollment through
// PublishedWorkstreamID before anything is published, so hooks and the MCP
// server still only need to agree here — both hand the daemon the same handle
// and the daemon translates them the same way.
func WorkstreamIDFor(vendor, sessionID string) (workstreamID, sessionAlias string, ok bool) {
	if vendor != "codex" && vendor != "claude" && vendor != "cursor" {
		return "", "", false
	}
	if sessionID == "" || len(sessionID) > 512 {
		return "", "", false
	}
	sum := sha256.Sum256([]byte("stickguy.agent-session.v1\x00" + vendor + "\x00" + sessionID))
	return fmt.Sprintf("wrk_agent_%x", sum[:16]), fmt.Sprintf("%s-%x", vendor, sum[:3]), true
}

// PublishedWorkstreamID scopes a locally derived agent-session handle to the
// enrollment it is being published into.
//
// WorkstreamIDFor is a pure function of (vendor, session id) so that every
// hook, adapter, and MCP process can derive it before it knows anything about
// a project. But the hosted service binds a workstream to one (project,
// workspace) pair, and an agent session routinely outlives a re-enrollment:
// publishing the unscoped handle into the new project collides with the
// binding the old project still holds, and the hosted service correctly
// refuses every event the session sends from then on (B24). Only the daemon
// knows which enrollment an observation belongs to, so it — and nothing
// upstream of it — performs this translation, and it must apply it uniformly:
// a brief is filtered by the workstream it is requested for, so an identity
// published one way and requested another would never see its own findings.
func PublishedWorkstreamID(workstreamID, projectID, workspaceID string) string {
	sum := sha256.Sum256([]byte("stickguy.agent-session-scope.v1\x00" + projectID + "\x00" + workspaceID + "\x00" + workstreamID))
	return fmt.Sprintf("wrk_agent_%x", sum[:16])
}

// ClassifyCoordinationTitle permits only the short vendor-visible label used
// for activity/v1 and automatic intent. It is deliberately stricter than local
// owner display because this value may leave the device and be embedded.
func ClassifyCoordinationTitle(value string) (string, error) {
	title := strings.Join(strings.Fields(value), " ")
	if title == "" || utf8.RuneCountInString(title) > 160 || strings.ContainsRune(title, '\x00') {
		return "", errors.New("session title is empty, invalid, or too large")
	}
	if privateKeyPattern.MatchString(title) || credentialPattern.MatchString(title) || environmentPattern.MatchString(title) || toolOutputPattern.MatchString(title) {
		return "", errors.New("session title contains prohibited content")
	}
	return title, nil
}

// ClassifyMessage is the final local boundary before a message may be shared.
// It rejects the entire candidate; it never redacts prohibited material into
// allowed content. Under ADR-036 quoted code and diffs are allowed, because an
// agent conversation is unreadable without them and the member explicitly chose
// to share it. Secrets, environment values, and raw tool output are not.
//
// ADR-038 narrows this to the material itself: referring to a credential file by
// name is ordinary conversation, so only actual values reject a message.
func ClassifyMessage(candidate Message) (Message, error) {
	if !shareableKinds[candidate.Kind] {
		return Message{}, errors.New("conversation message kind is unsupported")
	}
	text := strings.TrimSpace(candidate.Text)
	if text == "" || len(text) > MaxMessageBytes || !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
		return Message{}, errors.New("conversation message is empty, invalid, or too large")
	}
	if privateKeyPattern.MatchString(text) || credentialPattern.MatchString(text) ||
		environmentPattern.MatchString(text) || toolOutputPattern.MatchString(text) {
		return Message{}, errors.New("conversation message contains prohibited content")
	}
	return Message{Kind: candidate.Kind, Text: text}, nil
}

// ProhibitedContractSignature is the mandatory wire gate for derived contract
// signature text (ADR-038 semantics, ADR-044). A denied signature is dropped
// from the fingerprint entirely rather than redacted.
//
// It deliberately omits the environment-assignment pattern that guards prose:
// an exported declaration such as `const MAX_RETRIES = 3` is ordinary API
// surface, and rejecting it would silently blind contract comparison. Actual
// credential material in a declaration — an API key, a token, a private key —
// is still caught by the credential and private-key patterns.
func ProhibitedContractSignature(signature string) bool {
	return strings.ContainsRune(signature, '\x00') || privateKeyPattern.MatchString(signature) ||
		credentialPattern.MatchString(signature) || toolOutputPattern.MatchString(signature)
}

func NormalizePaths(event Event, repositoryRoot string) (Event, error) {
	root, err := filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return Event{}, fmt.Errorf("canonicalize registered repository: %w", err)
	}
	seen := map[string]bool{}
	paths := make([]string, 0, len(event.CandidatePaths))
	for _, candidate := range event.CandidatePaths {
		absolute := candidate
		if !filepath.IsAbs(absolute) {
			absolute = filepath.Join(event.CWD, absolute)
		}
		absolute = canonicalizeExisting(filepath.Clean(absolute))
		relative, err := filepath.Rel(root, absolute)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return Event{}, errors.New("activity path escapes the registered repository")
		}
		relative = filepath.ToSlash(relative)
		if err := validateSafePath(relative); err != nil {
			return Event{}, err
		}
		if !seen[relative] {
			seen[relative] = true
			paths = append(paths, relative)
		}
	}
	slices.Sort(paths)
	if len(paths) > 100 {
		return Event{}, errors.New("activity event exceeds the safe-path limit")
	}
	event.CandidatePaths = paths
	if len(paths) > 0 && (event.Kind == "PreToolUse" || event.Kind == "PostToolUse") {
		event.Action = toolLabel(event.Tool) + " " + paths[0]
		if len(paths) > 1 {
			event.Action += fmt.Sprintf(" and %d more", len(paths)-1)
		}
	}
	return event, nil
}

// canonicalizeExisting resolves symlinks in an absolute path so it can be
// compared against the already-resolved repository root. A vendor reports the
// path it was given, which on macOS routinely travels through a symlinked
// ancestor such as /tmp; comparing that textually against a resolved root
// rejects every path in the event and silently empties the safe-path set.
//
// The file itself need not exist — a write names a path before creating it — so
// resolution falls back to the deepest existing ancestor and re-joins the rest.
// Resolving before the containment check also narrows the boundary rather than
// widening it: a symlink pointing outside the repository now resolves outside
// and is rejected, where a textual comparison accepted it.
func canonicalizeExisting(absolute string) string {
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		return resolved
	}
	remainder := ""
	current := absolute
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return absolute
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(resolved, remainder)
		}
	}
}

func candidatePaths(tool string, input map[string]any) []string {
	var paths []string
	for _, key := range []string{"file_path", "path", "old_path", "new_path", "notebook_path"} {
		if value, ok := input[key].(string); ok && value != "" && len(value) <= 4096 {
			paths = append(paths, value)
		}
	}
	// Codex apply_patch carries paths in patch headers. Extract only header path
	// strings; patch/source content is discarded in this function invocation.
	if tool == "apply_patch" {
		if command, ok := input["command"].(string); ok && len(command) <= MaxInputBytes {
			for _, line := range strings.Split(command, "\n") {
				for _, prefix := range []string{"*** Add File: ", "*** Update File: ", "*** Delete File: ", "*** Move to: "} {
					if strings.HasPrefix(line, prefix) {
						paths = append(paths, strings.TrimSpace(strings.TrimPrefix(line, prefix)))
					}
				}
			}
		}
	}
	return paths
}

func validateSafePath(value string) error {
	if value == "" || value == "." || len(value) > 512 || strings.ContainsRune(value, '\x00') || strings.Contains(value, `\`) {
		return errors.New("activity path is invalid")
	}
	lower := strings.ToLower(value)
	segments := strings.Split(lower, "/")
	protected := map[string]bool{".ssh": true, ".aws": true, ".azure": true, ".kube": true, ".npmrc": true, ".pypirc": true, ".git-credentials": true, "credentials": true, "secrets": true, "id_rsa": true, "id_ed25519": true}
	for _, segment := range segments {
		if segment == ".env" || strings.HasPrefix(segment, ".env.") || protected[segment] || strings.HasSuffix(segment, ".pem") || strings.HasSuffix(segment, ".key") {
			return errors.New("activity event references a protected path")
		}
	}
	if strings.Contains(lower, "/.config/gcloud/") {
		return errors.New("activity event references a protected path")
	}
	return nil
}

// ReadTool reports whether a tool observation is a file inspection rather than
// a mutation. It matches exactly the tools toolLabel already categorizes as
// inspecting files, which is the read-set source under ADR-048.
func ReadTool(tool string) bool {
	switch strings.ToLower(tool) {
	case "read", "glob", "grep":
		return true
	}
	return false
}

// Read-set coverage classes, ordered by strength (ADR-052). These name the best
// evidence Stickguy can obtain for a session, not the evidence it happens to
// hold: a session with no reads yet still has the coverage its vendor allows.
const (
	CoverageNone           = "none"
	CoverageSelfDeclared   = "self_declared"
	CoverageVendorInferred = "vendor_inferred"
	CoverageObserved       = "observed"
)

// ReadCoverage reports the strongest read evidence available for a vendor's
// sessions. Claude names each file it reads to a file-reading tool, so its
// reads are observed.
//
// Codex has no file-reading tool: it inspects source through the shell, and a
// shell observation carries a command rather than a list of files, so no hook
// event ever names a file it read. MCP self-declaration can add exact paths
// when the local service can resolve one live Codex thread for the client's
// cwd. A checkout with zero or multiple candidates stays unidentified rather
// than assigning one session's reads to another.
//
// What does fill it, partially, is Codex's own classification of the commands
// it ran, recovered from the app-server at turn boundaries. That evidence is
// vendor-inferred and incomplete, so it is reported as such, and only when a
// Codex the device can actually talk to is present.
//
// inferredReadsAvailable means the mechanism is usable, not merely installed.
// The caller lowers it once a refresh for that session has demonstrably failed,
// because a Codex that is present but cannot answer recovers no reads at all,
// and claiming inferred coverage for it is the same silent overstatement as
// claiming it with no Codex installed.
//
// Cursor's beforeReadFile hook fires before a file is read and names that file
// in `file_path`, so its reads are observed the same way Claude's are — from the
// vendor's own statement of which file, not from an inference about a command.
//
// Reporting that honestly is the entire point. A session with no read coverage
// can never receive a stale_assumption finding, so silence for it means
// absence of evidence, not absence of drift.
func ReadCoverage(vendor string, inferredReadsAvailable bool) string {
	switch vendor {
	case "claude", "cursor":
		return CoverageObserved
	case "codex":
		if inferredReadsAvailable {
			return CoverageVendorInferred
		}
	}
	return CoverageNone
}

// SafeRepositoryPath reports whether a repository-relative path may be shared.
// It is the same rule NormalizePaths applies, exposed for callers that filter
// individual candidates instead of rejecting a whole observation.
func SafeRepositoryPath(value string) bool { return validateSafePath(value) == nil }

func toolAction(tool string, completed bool) string {
	verb := "Using"
	if completed {
		verb = "Finished"
	}
	return verb + " " + toolLabel(tool)
}

func toolLabel(tool string) string {
	switch strings.ToLower(tool) {
	case "edit", "write", "multiedit", "apply_patch", "notebookedit":
		return "editing"
	case "read", "glob", "grep":
		return "inspecting files"
	case "bash", "exec_command", "write_stdin":
		return "running a command"
	case "agent", "task":
		return "using a subagent"
	case "websearch", "webfetch":
		return "researching"
	case "":
		return "a tool"
	default:
		return "tool " + tool
	}
}

func agentLabel(agentType string) string {
	if agentType == "" {
		return "subagent"
	}
	return agentType + " subagent"
}
