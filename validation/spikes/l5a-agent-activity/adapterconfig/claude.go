package adapterconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const managedPrefix = "stickguy activity-hook "

var hookEvents = []string{
	"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse",
	"SubagentStart", "SubagentStop", "Stop", "SessionEnd",
}

type hook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type group struct {
	Matcher string `json:"matcher,omitempty"`
	Hooks   []hook `json:"hooks"`
}

func InstallClaude(input []byte) ([]byte, error) {
	document, hooks, err := parse(input)
	if err != nil {
		return nil, err
	}
	for _, event := range hookEvents {
		expected := expectedGroup(event)
		groups, err := decodeGroups(hooks[event])
		if err != nil {
			return nil, fmt.Errorf("decode %s hooks: %w", event, err)
		}
		for _, existing := range groups {
			for _, existingHook := range existing.Hooks {
				if strings.HasPrefix(existingHook.Command, managedPrefix) && existingHook.Command != expected.Hooks[0].Command {
					return nil, errors.New("managed Claude activity hook drifted")
				}
				if existingHook.Command == expected.Hooks[0].Command {
					return nil, errors.New("managed Claude activity hook already installed")
				}
			}
		}
		groups = append(groups, expected)
		hooks[event], err = json.Marshal(groups)
		if err != nil {
			return nil, err
		}
	}
	document["hooks"], err = json.Marshal(hooks)
	if err != nil {
		return nil, err
	}
	return encode(document)
}

func RemoveClaude(input []byte) ([]byte, error) {
	document, hooks, err := parse(input)
	if err != nil {
		return nil, err
	}
	for _, event := range hookEvents {
		expected := expectedGroup(event)
		groups, err := decodeGroups(hooks[event])
		if err != nil {
			return nil, err
		}
		kept := groups[:0]
		found := false
		for _, existing := range groups {
			managed := false
			for _, existingHook := range existing.Hooks {
				if strings.HasPrefix(existingHook.Command, managedPrefix) {
					if existingHook.Command != expected.Hooks[0].Command {
						return nil, errors.New("managed Claude activity hook drifted; refusing removal")
					}
					managed = true
					found = true
				}
			}
			if !managed {
				kept = append(kept, existing)
			}
		}
		if !found {
			return nil, errors.New("managed Claude activity hook missing")
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event], err = json.Marshal(kept)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(hooks) == 0 {
		delete(document, "hooks")
	} else {
		document["hooks"], err = json.Marshal(hooks)
		if err != nil {
			return nil, err
		}
	}
	return encode(document)
}

func expectedGroup(event string) group {
	matcher := ""
	if event == "PreToolUse" || event == "PostToolUse" {
		matcher = "Read|Bash|Agent|Task"
	}
	return group{Matcher: matcher, Hooks: []hook{{Type: "command", Command: managedPrefix + event}}}
}

func parse(input []byte) (map[string]json.RawMessage, map[string]json.RawMessage, error) {
	if len(input) == 0 {
		input = []byte("{}")
	}
	if len(input) > 1<<20 {
		return nil, nil, errors.New("Claude settings exceed 1 MiB")
	}
	document := map[string]json.RawMessage{}
	if err := json.Unmarshal(input, &document); err != nil {
		return nil, nil, fmt.Errorf("parse Claude settings: %w", err)
	}
	hooks := map[string]json.RawMessage{}
	if raw := document["hooks"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, nil, errors.New("Claude hooks must be an object")
		}
	}
	return document, hooks, nil
}

func decodeGroups(raw json.RawMessage) ([]group, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var groups []group
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func encode(document map[string]json.RawMessage) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
