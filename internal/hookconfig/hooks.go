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

// codexSessionEndTimeout is the ceiling Codex enforces on a synchronous
// SessionEnd handler. Exceeding it is not an error: Codex silently lowers the
// value and warns, which is worse than matching it.
const codexSessionEndTimeout = 3

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

// Install structurally adds Overgent's exact managed hooks while preserving all
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
						return errors.New("managed Overgent activity hook drifted; refusing to overwrite it")
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
		for groupIndex := range groups {
			for handlerIndex, candidate := range groups[groupIndex].Hooks {
				if managed(candidate.Command) {
					if candidate.Command != command {
						return errors.New("managed Overgent activity hook drifted; refusing to overwrite it")
					}
					// The command matched exactly, so this handler is this
					// profile's own. Its tuning is Overgent's to set: repair it
					// rather than refusing the file. Refusing here protected
					// nothing and stranded the member, because a hand-edited
					// timeout is indistinguishable from an older Overgent's.
					wanted := expected(event, command).Hooks[0]
					if candidate != wanted {
						groups[groupIndex].Hooks[handlerIndex] = wanted
						hooks[event], err = json.Marshal(groups)
						if err != nil {
							return err
						}
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
// from a structurally valid Overgent binding owned by another local profile.
// Unknown or conflicting managed-looking commands fail closed.
func Inspect(path, command string) (Inspection, error) {
	_, hooks, err := read(path)
	if err != nil {
		return Inspection{}, err
	}
	vendor := commandVendor(command)
	if vendor == "" {
		return Inspection{}, errors.New("invalid expected Overgent activity hook command")
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
						return Inspection{}, errors.New("managed Overgent activity hook drifted")
					}
					existingCommands[candidate.Command] = true
					// Only the command decides ownership; the vendor check above
					// already made that call. A handler of ours whose tuning
					// differs — an older Overgent's, or one a member edited — is
					// incomplete, not foreign, so it reports partial and Install
					// repairs it. Reporting drift here was a dead end: the
					// desktop row offers reconnect only for another profile.
					if candidate == expected(event, candidate.Command).Hooks[0] {
						present[event] = true
					}
				}
			}
		}
	}
	if len(existingCommands) == 0 {
		return Inspection{State: BindingNotConfigured}, nil
	}
	if len(existingCommands) != 1 {
		return Inspection{}, errors.New("conflicting managed Overgent activity hooks")
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

// Rebind replaces only structurally recognized Overgent hooks for this vendor.
// Unrelated hook groups and handlers are preserved byte-semantically through
// JSON decoding/re-encoding, and unknown managed-looking commands fail closed.
func Rebind(path, command string) error {
	document, hooks, err := read(path)
	if err != nil {
		return err
	}
	vendor := commandVendor(command)
	if vendor == "" {
		return errors.New("invalid Overgent activity hook command")
	}
	inspection, err := Inspect(path, command)
	if err != nil {
		return err
	}
	if inspection.State != BindingCurrent && inspection.State != BindingPartial && inspection.State != BindingOtherProfile && inspection.State != BindingNotConfigured {
		return errors.New("unsupported Overgent activity hook binding")
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
						return errors.New("managed Overgent activity hook drifted; refusing rebind")
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
						return errors.New("managed Overgent activity hook drifted; refusing removal")
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

// Command builds the managed hook command for a vendor.
//
// Cursor is accepted here because it invokes the same executable through the
// same `agent-hook --vendor` entry point, but it is NOT accepted by this
// package's file operations: Cursor's configuration is a different document
// (`.cursor/hooks.json`, a versioned object keyed by camelCase event name with
// its own per-handler fields) rather than the Claude/Codex settings shape these
// functions read and write. internal/cursorsetup owns that file and appends the
// `--event` argument each Cursor hook needs, because two of Cursor's events name
// themselves nowhere in their payload.
func Command(executable, configRoot, vendor string) (string, error) {
	if !supportedVendor(vendor) {
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
	if !supportedVendor(vendor) {
		return "", errors.New("unsupported activity-hook vendor")
	}
	return strings.Join([]string{shellQuote("overgent"), "agent-hook", "--vendor", vendor}, " "), nil
}

func expected(event, command string) group {
	matcher := ""
	if event == "PreToolUse" || event == "PermissionRequest" || event == "PostToolUse" || event == "PostToolUseFailure" {
		matcher = "*"
	}
	// Context-bearing turn-boundary hooks must complete synchronously so their
	// additionalContext reaches the triggering turn. Every other observation
	// hook preserves the existing asynchronous behavior; SessionEnd remains
	// synchronous because Codex requires it.
	injectionBoundary := event == "SessionStart" || event == "UserPromptSubmit"
	timeout := 5
	if injectionBoundary {
		timeout = 2
	}
	// Codex caps a synchronous SessionEnd at codexSessionEndTimeout and prints
	// "clamping SessionEnd hook timeout to 3s in <path>" every time it has to,
	// so asking for 5 bought nothing and put an Overgent-owned filename in front
	// of the member on every session they closed. Writing the cap is also what
	// Codex hashes for hook trust — it normalizes before hashing (ADR-051) — so
	// a binding already trusted at 5 stays trusted through this change.
	if event == "SessionEnd" && commandVendor(command) == "codex" {
		timeout = codexSessionEndTimeout
	}
	return group{Matcher: matcher, Hooks: []handler{{Type: "command", Command: command, Async: event != "SessionEnd" && !injectionBoundary, Timeout: timeout}}}
}

func managed(command string) bool {
	return strings.Contains(command, " agent-hook --vendor ")
}

func supportedVendor(vendor string) bool {
	return vendor == "codex" || vendor == "claude" || vendor == "cursor"
}

// commandVendor names the vendor of a managed command found in a Claude or Codex
// settings file. Cursor is absent on purpose: a Cursor command has no business
// in either of those documents, and returning "" makes every caller treat it as
// unrecognized drift and refuse to touch the file, rather than rewriting it as
// though it were a Claude hook.
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
	return prefix == "'overgent'" || strings.HasPrefix(prefix, "'") && strings.Contains(prefix, "' --config-root '")
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
	temporary, err := os.CreateTemp(filepath.Dir(path), ".overgent-hooks-*")
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

// OvergentMCPTools are the coordination tools `overgent mcp` registers, in the
// permission form Claude Code matches. The MCP server is the source of truth
// for this list; a test there asserts the two agree, so adding a tool without
// pre-approving it fails in CI rather than at a member's keyboard.
var OvergentMCPTools = []string{
	"mcp__overgent__acknowledge_context",
	"mcp__overgent__begin_work",
	"mcp__overgent__check_coordination",
	"mcp__overgent__finish_work",
	"mcp__overgent__get_resolutions",
	"mcp__overgent__report_checkpoint",
	"mcp__overgent__report_event",
	"mcp__overgent__update_intent",
}

// AllowTools pre-approves exactly Overgent's own coordination tools in the
// settings file this package already manages.
//
// A member who has just run `setup claude` is then asked to approve begin_work,
// check_coordination, report_checkpoint and finish_work one at a time, which is
// four interruptions from the coordination layer before any work happens — and
// a coordination harness that interrupts more than it prevents is worse than
// none. These tools publish bounded intent and read back a brief; they do not
// edit files, run commands, or reach outside the Project.
//
// The grant is deliberately narrow. Only the exact tool names above are added,
// never a wildcard and never another server's tools, entries a member already
// wrote are preserved, and `Remove` withdraws exactly what was granted.
func AllowTools(path string, tools []string) error {
	document, hooks, err := read(path)
	if err != nil {
		return err
	}
	allow, permissions, err := allowList(document)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for _, entry := range allow {
		existing[entry] = true
	}
	for _, tool := range tools {
		if !existing[tool] {
			allow = append(allow, tool)
			existing[tool] = true
		}
	}
	if err = setAllowList(document, permissions, allow); err != nil {
		return err
	}
	return write(path, document, hooks)
}

// DisallowTools withdraws exactly the entries AllowTools granted, leaving every
// other permission a member configured untouched.
func DisallowTools(path string, tools []string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	document, hooks, err := read(path)
	if err != nil {
		return err
	}
	allow, permissions, err := allowList(document)
	if err != nil {
		return err
	}
	granted := map[string]bool{}
	for _, tool := range tools {
		granted[tool] = true
	}
	kept := make([]string, 0, len(allow))
	for _, entry := range allow {
		if !granted[entry] {
			kept = append(kept, entry)
		}
	}
	if err = setAllowList(document, permissions, kept); err != nil {
		return err
	}
	return write(path, document, hooks)
}

func allowList(document map[string]json.RawMessage) ([]string, map[string]json.RawMessage, error) {
	permissions := map[string]json.RawMessage{}
	if raw := document["permissions"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &permissions); err != nil {
			return nil, nil, errors.New("permissions must be a JSON object")
		}
	}
	var allow []string
	if raw := permissions["allow"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &allow); err != nil {
			return nil, nil, errors.New("permissions.allow must be an array of strings")
		}
	}
	return allow, permissions, nil
}

func setAllowList(document, permissions map[string]json.RawMessage, allow []string) error {
	if len(allow) == 0 {
		delete(permissions, "allow")
	} else {
		raw, err := json.Marshal(allow)
		if err != nil {
			return err
		}
		permissions["allow"] = raw
	}
	if len(permissions) == 0 {
		delete(document, "permissions")
		return nil
	}
	raw, err := json.Marshal(permissions)
	if err != nil {
		return err
	}
	document["permissions"] = raw
	return nil
}

// ParseManagedCommand splits a managed Overgent hook command into the
// executable it runs and the Overgent profile it is bound to. A portable
// command names no profile, so it reports an empty config root.
//
// It fails closed. Anything that is not exactly the shape Command and
// PortableCommand write is not ours to interpret, and every caller treats
// ok=false as "leave this alone".
func ParseManagedCommand(command string) (executable, configRoot string, ok bool) {
	if strings.ContainsAny(command, "\r\n\x00") {
		return "", "", false
	}
	index := strings.Index(command, " agent-hook --vendor ")
	if index < 0 {
		return "", "", false
	}
	prefix := command[:index]
	if prefix == "'overgent'" {
		return "overgent", "", true
	}
	const marker = "' --config-root '"
	at := strings.Index(prefix, marker)
	if at < 0 || !strings.HasPrefix(prefix, "'") || !strings.HasSuffix(prefix, "'") {
		return "", "", false
	}
	executable, executableOK := unquoteShell(prefix[:at+1])
	configRoot, rootOK := unquoteShell(prefix[at+len(marker)-1:])
	if !executableOK || !rootOK || !filepath.IsAbs(executable) || !filepath.IsAbs(configRoot) {
		return "", "", false
	}
	return executable, filepath.Clean(configRoot), true
}

func unquoteShell(value string) (string, bool) {
	if len(value) < 2 || !strings.HasPrefix(value, "'") || !strings.HasSuffix(value, "'") {
		return "", false
	}
	return strings.ReplaceAll(value[1:len(value)-1], `'\''`, "'"), true
}

// legacyProfileNames are product names this application used before it was
// called Overgent. A profile directory with one of these exact names was
// written by an earlier build of this same product, for this same member.
//
// The list is exact names rather than a pattern on purpose (ADR-065): the point
// is to recognize our own former shapes, and a pattern could match a directory
// belonging to somebody else's software.
var legacyProfileNames = map[string]bool{"Stickguy": true}

// Abandoned reports whether a managed binding owned by previousProfile can be
// adopted onto currentConfigRoot without asking the member first.
//
// The question this answers is not "is it ours" - every binding this package
// recognizes is ours - but "is anything still using it". A binding nothing can
// still be using is not a decision, it is a leftover, and presenting it as a
// decision is what stranded members behind a Reconnect button that named the
// same profile on both sides of the arrow.
//
// It is deliberately conservative: an unrecognized previous executable, or a
// profile that still exists and still holds an enrolled device, is left for an
// explicit reconnect. False here costs a confirmation; true here silently moves
// a binding that another live profile depends on.
func Abandoned(currentConfigRoot, previousProfile, previousExecutable string) bool {
	current := cleanProfile(currentConfigRoot)
	previous := cleanProfile(previousProfile)
	switch {
	case previous == "":
		// A portable binding resolves `overgent` on PATH. It names no profile to
		// take anything from, and on this Mac that PATH entry is this member's.
		return true
	case previous == current:
		// One profile, two executable paths: an app bundle that was rebuilt or
		// moved, or a CLI that has since been copied to ~/.local/bin. Reporting
		// this as another profile is how a member ends up being asked to confirm
		// moving a binding from a profile to itself.
		return true
	case !directoryExists(previous):
		// The profile it points at is gone, so nothing can be running from it.
		return true
	case legacyProfileNames[filepath.Base(previous)]:
		// A superseded product name, written by an earlier build of this same
		// product for this same member.
		return true
	case !profileHasDevice(previous):
		// The profile exists but was never enrolled, or has already been reset.
		// No device identity depends on the binding.
		return true
	case previousExecutable != "" && previousExecutable != "overgent" && !executableExists(previousExecutable):
		// Whatever profile it names, the binary it runs is gone, so the binding
		// can never fire again.
		return true
	default:
		return false
	}
}

func cleanProfile(value string) string {
	if value == "" || value == "portable" {
		return ""
	}
	if absolute, err := filepath.Abs(value); err == nil {
		return filepath.Clean(absolute)
	}
	return filepath.Clean(value)
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func executableExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// profileHasDevice reports whether an Overgent profile directory holds an
// enrolled device. It reads the file directly rather than through
// internal/config so that this predicate stays free of that package's platform
// gate and of any dependency the setup packages do not already have.
func profileHasDevice(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil {
		return false
	}
	var stored struct {
		DeviceID string `json:"deviceId"`
	}
	return json.Unmarshal(data, &stored) == nil && stored.DeviceID != ""
}
