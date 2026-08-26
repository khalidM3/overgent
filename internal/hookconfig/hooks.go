package hookconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var claudeEvents = []string{
	"SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest",
	"PostToolUse", "PostToolUseFailure", "SubagentStart", "SubagentStop", "Stop", "SessionEnd",
}

var codexEvents = []string{
	"SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest",
	"PostToolUse", "SubagentStart", "SubagentStop", "Stop", "SessionEnd",
}

type handler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Async   bool   `json:"async,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

type group struct {
	Matcher string    `json:"matcher,omitempty"`
	Hooks   []handler `json:"hooks"`
}

type BindingState string

const (
	BindingNotConfigured BindingState = "not_configured"
	BindingCurrent       BindingState = "current"
	BindingPartial       BindingState = "partial"
	BindingOtherProfile  BindingState = "other_profile"
)

type Inspection struct {
	State           BindingState
	ExistingCommand string
}

// Install structurally adds Stickguy's exact managed hooks while preserving all
// unrelated settings and hooks. The command is intentionally a fixed string
// assembled from application-owned absolute paths, never user input.
func Install(path, command string) error {
	document, hooks, err := read(path)
	if err != nil {
		return err
	}
	if strings.HasSuffix(command, "--vendor codex") {
		groups, decodeErr := groupsFor(hooks["PostToolUseFailure"])
		if decodeErr != nil {
			return fmt.Errorf("decode PostToolUseFailure hooks: %w", decodeErr)
		}
		kept := make([]group, 0, len(groups))
		for _, existing := range groups {
			remove := false
			for _, candidate := range existing.Hooks {
				if managed(candidate.Command) {
					if candidate.Command != command {
						return errors.New("managed Stickguy activity hook drifted; refusing to overwrite it")
					}
					remove = true
				}
			}
			if !remove {
				kept = append(kept, existing)
			}
		}
		if len(kept) == 0 {
			delete(hooks, "PostToolUseFailure")
		} else if hooks["PostToolUseFailure"], err = json.Marshal(kept); err != nil {
			return err
		}
	}
	for _, event := range configuredEvents(command) {
		groups, err := groupsFor(hooks[event])
		if err != nil {
			return fmt.Errorf("decode %s hooks: %w", event, err)
		}
		for _, existing := range groups {
			for _, candidate := range existing.Hooks {
				if managed(candidate.Command) {
					if candidate.Command != command {
						return errors.New("managed Stickguy activity hook drifted; refusing to overwrite it")
					}
					goto nextEvent
				}
			}
		}
		groups = append(groups, expected(event, command))
		hooks[event], err = json.Marshal(groups)
		if err != nil {
			return err
		}
	nextEvent:
	}
	return write(path, document, hooks)
}

func Status(path, command string) (bool, error) {
	inspection, err := Inspect(path, command)
	return inspection.State == BindingCurrent, err
}

// Inspect distinguishes a complete current binding from a partial install and
// from a structurally valid Stickguy binding owned by another local profile.
// Unknown or conflicting managed-looking commands fail closed.
func Inspect(path, command string) (Inspection, error) {
	_, hooks, err := read(path)
	if err != nil {
		return Inspection{}, err
	}
	vendor := commandVendor(command)
	if vendor == "" {
		return Inspection{}, errors.New("invalid expected Stickguy activity hook command")
	}
	existingCommands := map[string]bool{}
	present := map[string]bool{}
	for _, event := range configuredEvents(command) {
		groups, err := groupsFor(hooks[event])
		if err != nil {
			return Inspection{}, fmt.Errorf("decode %s hooks: %w", event, err)
		}
		for _, existing := range groups {
			for _, candidate := range existing.Hooks {
				if managed(candidate.Command) {
					if !managedForVendor(candidate.Command, vendor) {
						return Inspection{}, errors.New("managed Stickguy activity hook drifted")
					}
					existingCommands[candidate.Command] = true
					present[event] = true
				}
			}
		}
	}
	if len(existingCommands) == 0 {
		return Inspection{State: BindingNotConfigured}, nil
	}
	if len(existingCommands) != 1 {
		return Inspection{}, errors.New("conflicting managed Stickguy activity hooks")
	}
	var existingCommand string
	for value := range existingCommands {
		existingCommand = value
	}
	if existingCommand != command {
		return Inspection{State: BindingOtherProfile, ExistingCommand: existingCommand}, nil
	}
	if len(present) != len(configuredEvents(command)) {
		return Inspection{State: BindingPartial, ExistingCommand: existingCommand}, nil
	}
	return Inspection{State: BindingCurrent, ExistingCommand: existingCommand}, nil
}

// Rebind replaces only structurally recognized Stickguy hooks for this vendor.
// Unrelated hook groups and handlers are preserved byte-semantically through
// JSON decoding/re-encoding, and unknown managed-looking commands fail closed.
func Rebind(path, command string) error {
	document, hooks, err := read(path)
	if err != nil {
		return err
	}
	vendor := commandVendor(command)
	if vendor == "" {
		return errors.New("invalid Stickguy activity hook command")
	}
	inspection, err := Inspect(path, command)
	if err != nil {
		return err
	}
	if inspection.State != BindingCurrent && inspection.State != BindingPartial && inspection.State != BindingOtherProfile && inspection.State != BindingNotConfigured {
		return errors.New("unsupported Stickguy activity hook binding")
	}
	for event, raw := range hooks {
		groups, decodeErr := groupsFor(raw)
		if decodeErr != nil {
			return fmt.Errorf("decode %s hooks: %w", event, decodeErr)
		}
		keptGroups := make([]group, 0, len(groups))
		for _, existing := range groups {
			keptHandlers := make([]handler, 0, len(existing.Hooks))
			for _, candidate := range existing.Hooks {
				if managed(candidate.Command) {
					if !managedForVendor(candidate.Command, vendor) {
						return errors.New("managed Stickguy activity hook drifted; refusing rebind")
					}
					continue
				}
				keptHandlers = append(keptHandlers, candidate)
			}
			if len(keptHandlers) > 0 {
				existing.Hooks = keptHandlers
				keptGroups = append(keptGroups, existing)
			}
		}
		if len(keptGroups) == 0 {
			delete(hooks, event)
		} else if hooks[event], err = json.Marshal(keptGroups); err != nil {
			return err
		}
	}
	for _, event := range configuredEvents(command) {
		groups, decodeErr := groupsFor(hooks[event])
		if decodeErr != nil {
			return fmt.Errorf("decode %s hooks: %w", event, decodeErr)
		}
		groups = append(groups, expected(event, command))
		if hooks[event], err = json.Marshal(groups); err != nil {
			return err
		}
	}
	return write(path, document, hooks)
}

func Remove(path, command string) error {
	document, hooks, err := read(path)
	if err != nil {
		return err
	}
	for _, event := range configuredEvents(command) {
		groups, err := groupsFor(hooks[event])
		if err != nil {
			return fmt.Errorf("decode %s hooks: %w", event, err)
		}
		kept := make([]group, 0, len(groups))
		for _, existing := range groups {
			remove := false
			for _, candidate := range existing.Hooks {
				if managed(candidate.Command) {
					if candidate.Command != command {
						return errors.New("managed Stickguy activity hook drifted; refusing removal")
					}
					remove = true
				}
			}
			if !remove {
				kept = append(kept, existing)
			}
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event], err = json.Marshal(kept)
			if err != nil {
				return err
			}
		}
	}
	return write(path, document, hooks)
}

func configuredEvents(command string) []string {
	if strings.HasSuffix(command, "--vendor codex") {
		return codexEvents
	}
	return claudeEvents
}

func Command(executable, configRoot, vendor string) (string, error) {
	if vendor != "codex" && vendor != "claude" {
		return "", errors.New("unsupported activity-hook vendor")
	}
	executable, err := filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	configRoot, err = filepath.Abs(configRoot)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{shellQuote(executable), "--config-root", shellQuote(configRoot), "agent-hook", "--vendor", vendor}, " "), nil
}

func PortableCommand(vendor string) (string, error) {
	if vendor != "codex" && vendor != "claude" {
		return "", errors.New("unsupported activity-hook vendor")
	}
	return strings.Join([]string{shellQuote("stickguy"), "agent-hook", "--vendor", vendor}, " "), nil
}

func expected(event, command string) group {
	matcher := ""
	if event == "PreToolUse" || event == "PermissionRequest" || event == "PostToolUse" || event == "PostToolUseFailure" {
		matcher = "*"
	}
	// SessionEnd is synchronous in Codex even when async is requested; the hook
	// command itself only performs a short loopback IPC call.
	return group{Matcher: matcher, Hooks: []handler{{Type: "command", Command: command, Async: event != "SessionEnd", Timeout: 5}}}
}

func managed(command string) bool {
	return strings.Contains(command, " agent-hook --vendor ")
}

func commandVendor(command string) string {
	for _, vendor := range []string{"codex", "claude"} {
		if strings.HasSuffix(command, " agent-hook --vendor "+vendor) {
			return vendor
		}
	}
	return ""
}

func managedForVendor(command, vendor string) bool {
	if commandVendor(command) != vendor || strings.ContainsAny(command, "\r\n\x00") {
		return false
	}
	prefix := strings.TrimSuffix(command, " agent-hook --vendor "+vendor)
	return prefix == "'stickguy'" || strings.HasPrefix(prefix, "'") && strings.Contains(prefix, "' --config-root '")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func read(path string) (map[string]json.RawMessage, map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data = []byte("{}")
	} else if err != nil {
		return nil, nil, fmt.Errorf("read hook settings: %w", err)
	}
	if len(data) > 1<<20 {
		return nil, nil, errors.New("hook settings exceed 1 MiB")
	}
	document := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, nil, fmt.Errorf("parse hook settings: %w", err)
	}
	hooks := map[string]json.RawMessage{}
	if raw := document["hooks"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, nil, errors.New("hooks must be a JSON object")
		}
	}
	return document, hooks, nil
}

func groupsFor(raw json.RawMessage) ([]group, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var groups []group
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func write(path string, document, hooks map[string]json.RawMessage) error {
	if len(hooks) == 0 {
		delete(document, "hooks")
	} else {
		raw, err := json.Marshal(hooks)
		if err != nil {
			return err
		}
		document["hooks"] = raw
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create hook settings directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".stickguy-hooks-*")
	if err != nil {
		return fmt.Errorf("create temporary hook settings: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err = temporary.Chmod(0o644); err == nil {
		_, err = temporary.Write(output.Bytes())
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write hook settings: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("activate hook settings: %w", err)
	}
	return nil
}
