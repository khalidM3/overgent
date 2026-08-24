package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type hookInput struct {
	HookEventName string `json:"hook_event_name"`
	CWD           string `json:"cwd"`
	Source        string `json:"source,omitempty"`
	AgentType     string `json:"agent_type,omitempty"`
	// All other fields, including transcript_path, are intentionally ignored.
}

func main() {
	dec := json.NewDecoder(os.Stdin)
	dec.DisallowUnknownFields()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		visibleFailure("invalid hook input")
		return
	}
	allowed := map[string]bool{"hook_event_name": true, "cwd": true, "source": true, "agent_type": true, "session_id": true, "turn_id": true, "agent_id": true, "permission_mode": true, "transcript_path": true}
	for k := range raw {
		if !allowed[k] {
			visibleFailure("unexpected hook input field")
			return
		}
	}
	var in hookInput
	b, _ := json.Marshal(raw)
	if err := json.Unmarshal(b, &in); err != nil {
		visibleFailure("invalid hook input")
		return
	}
	if in.HookEventName != "SessionStart" && in.HookEventName != "SubagentStart" {
		visibleFailure("unsupported hook event")
		return
	}
	if in.CWD == "" {
		visibleFailure("workspace resolution unavailable")
		return
	}
	// Gate-only evidence: record only the allowlisted event name, never hook input.
	if len(os.Args) == 3 && os.Args[1] == "--marker" {
		_ = os.WriteFile(os.Args[2], []byte(in.HookEventName+"\n"), 0o600)
	}
	context := "Stickguy fixture brief brf_fixture_1 at context revision 7. Fidelity: fixture_structural. Before broad/shared edits call check_coordination. No source, prompt, transcript, environment, command, patch, or tool output was read."
	out := map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": in.HookEventName, "additionalContext": context}}
	_ = json.NewEncoder(os.Stdout).Encode(out)
}

func visibleFailure(message string) {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"systemMessage": "Stickguy context unavailable: " + message})
	fmt.Fprintln(os.Stderr, "stickguy hook degraded:", message)
}
