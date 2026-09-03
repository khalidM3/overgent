package agentactivity

import (
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
)

// Cursor's hook payload is not Claude's, and widening the vendor allowlist over
// the existing decoder would have produced a session with no identity, no
// working directory, and no event name (ADR-039: each vendor gets its own
// adapter).
//
// Four differences drive everything in this file:
//
//   - The join key is `conversation_id`. `session_id` exists only on
//     sessionStart/sessionEnd, so deriving workstream identity from it would
//     give the same chat a different workstream on every hook that omits it.
//   - The working directory arrives as `workspace_roots`, an array, and only on
//     sessionStart.
//   - Event names are camelCase, and `afterFileEdit` and `beforeSubmitPrompt`
//     carry no `hook_event_name` at all. The event is therefore declared by the
//     managed hook command rather than read from the payload, and a payload that
//     does name itself must agree with that declaration.
//   - `beforeReadFile` puts the whole file on stdin and `beforeSubmitPrompt`
//     puts the raw prompt there. Neither may become an Event field.
const (
	// MaxCursorInputBytes bounds Cursor's stdin. It is far larger than
	// MaxInputBytes because beforeReadFile carries the entire file being read,
	// and the 256 KiB bound would have rejected every read of a large file —
	// silently emptying the read set for exactly the files most worth tracking,
	// while reporting coverage as `observed`.
	//
	// The larger bound costs no memory: the decoder streams, and `content` has
	// no destination field, so encoding/json skips the value instead of
	// materializing it. What is bounded here is how much of a hostile or
	// runaway stdin will be walked, not how much is retained.
	MaxCursorInputBytes = 64 << 20

	// CursorWorkspaceRootEnv is the session-scoped variable Overgent publishes
	// from sessionStart's `env` output. Cursor passes it to every later hook in
	// the session, which is the only way afterFileEdit and beforeSubmitPrompt —
	// which carry no workspace root — can be attributed to a repository.
	CursorWorkspaceRootEnv = "OVERGENT_CURSOR_WORKSPACE_ROOT"
)

// cursorLifecycle projects one Cursor hook onto the lifecycle contract the
// coordination harness already speaks. Only the events Cursor is known to emit
// and Overgent can interpret appear here; anything else is rejected rather than
// assigned a guessed status, because an invented lifecycle kind is worse than a
// missing one.
type cursorLifecycle struct{ kind, status, action, tool string }

var cursorEvents = map[string]cursorLifecycle{
	"sessionStart":       {"SessionStart", "active", "Session started", ""},
	"beforeSubmitPrompt": {"UserPromptSubmit", "active", "Working on a new request", ""},
	"beforeReadFile":     {"PreToolUse", "active", toolAction("read", false), "read"},
	"afterFileEdit":      {"PostToolUse", "active", toolAction("edit", true), "edit"},
	"stop":               {"Stop", "idle", "Turn finished", ""},
	"sessionEnd":         {"SessionEnd", "done", "Session ended", ""},
}

// CursorEvents lists the hooks the managed Cursor configuration installs, in the
// order they are written. Setup and the parser read the same list so a hook can
// never be installed for an event the parser would reject.
var CursorEvents = []string{"sessionStart", "beforeSubmitPrompt", "beforeReadFile", "afterFileEdit", "stop", "sessionEnd"}

// SupportedCursorEvent reports whether a declared event name is one Overgent
// installs and can interpret.
func SupportedCursorEvent(event string) bool {
	_, ok := cursorEvents[event]
	return ok
}

// cursorPayload is the decode target for every Cursor hook except
// beforeSubmitPrompt.
//
// It deliberately has no field for `content`. beforeReadFile sends the entire
// file being read, and a key with no destination field is skipped by
// encoding/json rather than decoded, so file content is dropped during decoding
// instead of being held in a structure and discarded later — the same exclusion
// internal/codexappserver/threads.go applies to `command`.
type cursorPayload struct {
	ConversationID string   `json:"conversation_id"`
	HookEventName  string   `json:"hook_event_name"`
	WorkspaceRoots []string `json:"workspace_roots"`
	FilePath       string   `json:"file_path"`
}

// cursorPromptPayload is used only for beforeSubmitPrompt. The raw prompt is
// classified into a bounded coordination title inside ParseCursor and never
// copied anywhere else: it reaches no Event field, no daemon request, and no
// event payload. Every other Cursor hook decodes through cursorPayload, which
// has no prompt field at all.
type cursorPromptPayload struct {
	cursorPayload
	Prompt string `json:"prompt"`
}

// ParseCursor decodes one Cursor hook. declaredEvent is the camelCase event name
// the managed hook command was installed for; sessionRoot is the workspace root
// Cursor passes back through the session-scoped environment, used when the
// payload carries none of its own.
func ParseCursor(declaredEvent string, input io.Reader, sessionRoot string) (Event, error) {
	lifecycle, ok := cursorEvents[declaredEvent]
	if !ok {
		return Event{}, errors.New("unsupported cursor hook event")
	}
	if input == nil {
		return Event{}, errors.New("cursor hook input is missing")
	}
	decoder := json.NewDecoder(io.LimitReader(input, MaxCursorInputBytes+1))
	var payload cursorPayload
	var title string
	if declaredEvent == "beforeSubmitPrompt" {
		var prompted cursorPromptPayload
		if err := decoder.Decode(&prompted); err != nil {
			return Event{}, errors.New("invalid cursor hook JSON")
		}
		payload = prompted.cursorPayload
		// Cursor writes no transcript Overgent can read, so the submitted prompt
		// is the only vendor-visible text that can seed this session's intent.
		// ADR-042's classifier is the gate: it normalizes, bounds to 160
		// characters, and rejects credentials, environment values, private keys
		// and raw tool output. A rejected or oversize prompt yields no title
		// rather than a truncated one, so a long prompt leaves the session
		// showing its alias — which is honest, where a first-160-characters
		// summary would not be.
		if classified, classifyErr := ClassifyCoordinationTitle(prompted.Prompt); classifyErr == nil {
			title = classified
		}
	} else if err := decoder.Decode(&payload); err != nil {
		return Event{}, errors.New("invalid cursor hook JSON")
	}

	// A payload that names itself must agree with the hook it was installed
	// under. Disagreement means the configuration and the client have diverged,
	// which is drift, not an event to interpret.
	if payload.HookEventName != "" && payload.HookEventName != declaredEvent {
		return Event{}, errors.New("cursor hook event name does not match its configured hook")
	}
	conversationID := payload.ConversationID
	if conversationID == "" || len(conversationID) > 512 {
		return Event{}, errors.New("cursor hook conversation identity is missing or invalid")
	}
	workstreamID, sessionAlias, ok := WorkstreamIDFor("cursor", conversationID)
	if !ok {
		return Event{}, errors.New("cursor hook conversation identity is missing or invalid")
	}

	roots := cursorRoots(payload.WorkspaceRoots)
	if len(roots) == 0 && sessionRoot != "" && len(sessionRoot) <= 4096 {
		roots = cursorRoots([]string{sessionRoot})
	}
	if len(roots) == 0 {
		// Without a root the event cannot be attributed to a repository. This is
		// the ordinary state of a session whose sessionStart never reached the
		// service, so it degrades to no observation rather than to a guess.
		return Event{}, errors.New("cursor hook has no workspace root")
	}
	canonical, err := filepath.Abs(roots[0])
	if err != nil {
		return Event{}, errors.New("cursor workspace root is invalid")
	}

	event := Event{
		Vendor: "cursor", CWD: canonical, CandidateRoots: roots,
		WorkstreamID:    workstreamID,
		VendorSessionID: conversationID,
		SessionAlias:    sessionAlias,
		Kind:            lifecycle.kind,
		Status:          lifecycle.status,
		Action:          lifecycle.action,
		Tool:            lifecycle.tool,
		SessionTitle:    title,
	}
	if path := payload.FilePath; path != "" && len(path) <= 4096 {
		event.CandidatePaths = []string{path}
	}
	return event, nil
}

// cursorRoots normalizes the workspace_roots array. Cursor reports one entry for
// a normal project and several for a multi-root workspace; the caller resolves
// which of them this device has actually registered.
func cursorRoots(candidates []string) []string {
	roots := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || len(candidate) > 4096 || strings.ContainsRune(candidate, '\x00') {
			continue
		}
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		roots = append(roots, candidate)
		if len(roots) == 16 {
			break
		}
	}
	return roots
}
