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
)

const MaxInputBytes = 256 << 10

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
}

var identifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._:-]{0,63}$`)

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
	sum := sha256.Sum256([]byte("stickguy.agent-session.v1\x00" + vendor + "\x00" + sessionID))
	event := Event{
		Vendor: vendor, CWD: canonicalCWD,
		WorkstreamID: fmt.Sprintf("wrk_agent_%x", sum[:16]),
		SessionAlias: fmt.Sprintf("%s-%x", vendor, sum[:3]),
		Kind:         kind,
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

	if toolInput, ok := raw["tool_input"].(map[string]any); ok {
		event.CandidatePaths = candidatePaths(tool, toolInput)
	}
	return event, nil
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
		absolute = filepath.Clean(absolute)
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
