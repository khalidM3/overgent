// Package cursorsetup installs, inspects, moves, and removes Stickguy's managed
// hooks in a Cursor project.
//
// It does not reuse internal/hookconfig. Cursor's configuration is a different
// document: `<project-root>/.cursor/hooks.json`, a versioned object whose
// `hooks` map is keyed by camelCase event name and whose handlers carry
// `matcher` and `failClosed` rather than Claude's `async`. Sharing one writer
// across both shapes would mean guessing a record format across vendors, which
// is what ADR-039 and docs/adapter-development.md forbid.
//
// Two Cursor events — afterFileEdit and beforeSubmitPrompt — carry no
// `hook_event_name` in their payload, so each installed command names its own
// event with `--event`. That argument is part of the managed command and is
// checked exactly during drift detection.
package cursorsetup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stickguy/stickguy/internal/agentactivity"
	"github.com/stickguy/stickguy/internal/hookconfig"
)

// hookTimeoutSeconds bounds one managed Cursor hook.
//
// UNVERIFIED UNIT: Cursor's `timeout` field is written here in seconds, matching
// every other hook system this repository configures. It has not been confirmed
// against a running Cursor client. If Cursor reads milliseconds, these hooks
// expire immediately and Stickguy observes nothing — it never blocks or delays a
// turn, because failClosed is false, so the failure mode is silent absence of
// observation. Status reporting therefore never claims a working binding from
// file presence alone; a Cursor adapter stays pending until a real accepted
// Cursor event is recorded for this profile.
const hookTimeoutSeconds = 5

// injectionTimeoutSeconds bounds the two hooks that can carry a correction back
// into the turn. They complete synchronously, so they get a tighter budget.
const injectionTimeoutSeconds = 2

// maxConfigBytes bounds the configuration document this package will parse.
const maxConfigBytes = 1 << 20

// schemaVersion is the `version` Stickguy writes and the only one it will edit.
// An unrecognized version is drift: the handler shape may have changed, and
// rewriting it blind could disable a member's own hooks.
const schemaVersion = 1

type handler struct {
	Command string `json:"command"`
	Type    string `json:"type"`
	Timeout int    `json:"timeout"`
	Matcher string `json:"matcher,omitempty"`
	// FailClosed is always false and is written explicitly rather than omitted.
	// Stickguy must never block, delay, or fail a Cursor turn (ADR-017); relying
	// on Cursor's default would make that guarantee depend on a default this
	// repository does not control.
	FailClosed bool `json:"failClosed"`
}

type document struct {
	Version int
	Hooks   map[string][]handler
	// rest holds every top-level key other than `version` and `hooks`, verbatim,
	// so an unknown Cursor setting survives a Stickguy install byte for byte.
	rest map[string]json.RawMessage
	// rawHooks holds the original bytes of every event, and modeled records
	// which of those this package successfully decoded. An event that is not
	// modeled is rewritten from its original bytes; a modeled event is rewritten
	// from Hooks, including when Hooks no longer has it because removal emptied
	// it. Without the separate set, deleting an event would silently restore it
	// from rawHooks.
	rawHooks map[string]json.RawMessage
	modeled  map[string]bool
}

type Status struct {
	Configured      bool   `json:"configured"`
	ConfigPath      string `json:"configPath"`
	Approval        string `json:"approval"`
	Binding         string `json:"binding"`
	PreviousProfile string `json:"previousProfile,omitempty"`
	CheckedProfile  string `json:"checkedProfile"`
	RestartRequired bool   `json:"restartRequired"`
}

type Manager struct {
	ProjectRoot, ConfigRoot, Executable string
	Portable                            bool
}

// Setup writes Stickguy's managed hooks, preserving every other key in the
// document and every hook the member configured themselves.
func (m Manager) Setup() (Status, error) {
	path, base, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	doc, err := read(path)
	if err != nil {
		return Status{}, err
	}
	inspection, err := inspect(doc, base)
	if err != nil {
		return Status{}, err
	}
	if inspection.state == bindingOtherProfile {
		return Status{Binding: string(bindingOtherProfile), CheckedProfile: m.checkedProfile(), ConfigPath: path},
			errors.New("Cursor is connected to another Stickguy profile; explicit reconnect is required")
	}
	install(doc, base)
	if err = write(path, doc); err != nil {
		return Status{}, err
	}
	return Status{
		Configured: true, ConfigPath: path, Approval: approval,
		Binding: string(bindingCurrent), CheckedProfile: m.checkedProfile(), RestartRequired: true,
	}, nil
}

func (m Manager) Status() (Status, error) {
	path, base, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	doc, err := read(path)
	if err != nil {
		return Status{}, err
	}
	inspection, err := inspect(doc, base)
	if err != nil {
		return Status{}, err
	}
	return Status{
		Configured:      inspection.state == bindingCurrent,
		ConfigPath:      path,
		Approval:        approval,
		Binding:         string(inspection.state),
		PreviousProfile: inspection.previousProfile,
		CheckedProfile:  m.checkedProfile(),
		RestartRequired: inspection.state != bindingNotConfigured,
	}, nil
}

// Rebind moves a binding owned by another local profile onto this one. The
// document is snapshotted first and restored on any partial failure, so a
// failed rebind never leaves a project with half of two profiles' hooks.
func (m Manager) Rebind() (Status, error) {
	path, base, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	doc, err := read(path)
	if err != nil {
		return Status{}, err
	}
	// Inspect before touching anything: unrecognized managed-looking drift must
	// refuse the rebind rather than be overwritten by it.
	if _, err = inspect(doc, base); err != nil {
		return Status{}, err
	}
	snapshot, err := capture(path)
	if err != nil {
		return Status{}, err
	}
	removeManaged(doc)
	install(doc, base)
	if err = write(path, doc); err != nil {
		if restoreErr := restore(path, snapshot); restoreErr != nil {
			return Status{}, fmt.Errorf("rebind Cursor hooks: %v; rollback: %w", err, restoreErr)
		}
		return Status{}, fmt.Errorf("rebind Cursor hooks: %w", err)
	}
	return Status{
		Configured: true, ConfigPath: path, Approval: approval,
		Binding: string(bindingCurrent), CheckedProfile: m.checkedProfile(), RestartRequired: true,
	}, nil
}

// Remove withdraws only Stickguy's own hooks. Unrelated keys, unrelated hooks,
// and any event Stickguy does not manage are left exactly as they were.
func (m Manager) Remove() (Status, error) {
	path, base, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	doc, err := read(path)
	if err != nil {
		return Status{}, err
	}
	if _, err = inspect(doc, base); err != nil {
		return Status{}, err
	}
	if removeManaged(doc) {
		if err = write(path, doc); err != nil {
			return Status{}, err
		}
	}
	return Status{Configured: false, ConfigPath: path, Approval: approval, Binding: string(bindingNotConfigured), CheckedProfile: m.checkedProfile()}, nil
}

// approval states what Stickguy actually knows about Cursor's consent step.
// docs/adapter-development.md requires establishing whether the vendor gates
// hook execution, and this has not been established against a real client, so
// the value says "unverified" instead of claiming Cursor runs what was written.
const approval = "unverified_by_cursor"

type bindingState string

const (
	bindingNotConfigured bindingState = "not_configured"
	bindingCurrent       bindingState = "current"
	bindingPartial       bindingState = "partial"
	bindingOtherProfile  bindingState = "other_profile"
)

type inspection struct {
	state           bindingState
	previousProfile string
}

func (m Manager) resolve() (string, string, error) {
	project, err := filepath.Abs(m.ProjectRoot)
	if err != nil {
		return "", "", fmt.Errorf("resolve project root: %w", err)
	}
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		return "", "", fmt.Errorf("canonicalize project root: %w", err)
	}
	if info, statErr := os.Stat(project); statErr != nil || !info.IsDir() {
		return "", "", errors.New("project root is not a directory")
	}
	var base string
	if m.Portable {
		base, err = hookconfig.PortableCommand("cursor")
	} else {
		base, err = hookconfig.Command(m.Executable, m.ConfigRoot, "cursor")
	}
	if err != nil {
		return "", "", err
	}
	return filepath.Join(project, ".cursor", "hooks.json"), base, nil
}

func (m Manager) checkedProfile() string {
	if m.Portable {
		return "portable"
	}
	if absolute, err := filepath.Abs(m.ConfigRoot); err == nil {
		return absolute
	}
	return m.ConfigRoot
}

// expected is the exact handler Stickguy installs for one event.
func expected(event, base string) handler {
	timeout := hookTimeoutSeconds
	// sessionStart carries the workspace root back through `env` and
	// beforeSubmitPrompt carries a correction back through additional_context.
	// Both must complete before the turn proceeds, so they run on the tight
	// budget rather than the observation budget.
	if event == "sessionStart" || event == "beforeSubmitPrompt" {
		timeout = injectionTimeoutSeconds
	}
	matcher := ""
	if event == "beforeReadFile" || event == "afterFileEdit" {
		matcher = "*"
	}
	return handler{Command: base + " --event " + event, Type: "command", Timeout: timeout, Matcher: matcher, FailClosed: false}
}

// managed reports whether a command is one of Stickguy's, by structure rather
// than by equality, so another profile's binding is recognized instead of being
// mistaken for a stranger's hook and silently duplicated.
func managed(command string) bool {
	return strings.Contains(command, " agent-hook --vendor cursor --event ")
}

// managedProfile returns the config root a managed command belongs to, or
// "portable" for a PATH install. It fails closed: a command that looks managed
// but does not have the exact shape this package writes returns ok=false, and
// every caller then refuses to touch the file.
func managedProfile(command, event string) (string, bool) {
	suffix := " agent-hook --vendor cursor --event " + event
	if !strings.HasSuffix(command, suffix) || strings.ContainsAny(command, "\r\n\x00") {
		return "", false
	}
	prefix := strings.TrimSuffix(command, suffix)
	if prefix == "'stickguy'" {
		return "portable", true
	}
	// The non-portable form is '<executable>' --config-root '<root>'.
	const marker = "' --config-root '"
	index := strings.Index(prefix, marker)
	if !strings.HasPrefix(prefix, "'") || index < 0 || !strings.HasSuffix(prefix, "'") {
		return "", false
	}
	root := prefix[index+len(marker) : len(prefix)-1]
	if root == "" || !filepath.IsAbs(root) {
		return "", false
	}
	return root, true
}

func inspect(doc *document, base string) (inspection, error) {
	if doc.Version != 0 && doc.Version != schemaVersion {
		return inspection{}, errors.New("Cursor hooks.json declares an unsupported schema version; refusing to edit it")
	}
	profiles := map[string]bool{}
	present := map[string]bool{}
	// A managed command under an event Stickguy does not install is drift: it
	// means an older or newer Stickguy wrote this file, and removing hooks by
	// event list alone would strand it.
	for event, handlers := range doc.Hooks {
		known := agentactivity.SupportedCursorEvent(event)
		for _, candidate := range handlers {
			if !managed(candidate.Command) {
				continue
			}
			if !known {
				return inspection{}, errors.New("managed Stickguy Cursor hook is configured for an unknown event; refusing to overwrite it")
			}
			profile, ok := managedProfile(candidate.Command, event)
			if !ok {
				return inspection{}, errors.New("managed Stickguy Cursor hook drifted; refusing to overwrite it")
			}
			profiles[profile] = true
			if candidate == expected(event, base) {
				present[event] = true
			} else if candidate.Command != expected(event, base).Command {
				// Another profile's command with this profile's handler fields,
				// or the reverse: both are recorded by profile above. Only a
				// same-command handler whose other fields drifted is a refusal.
				continue
			} else {
				return inspection{}, errors.New("managed Stickguy Cursor hook drifted; refusing to overwrite it")
			}
		}
	}
	if len(profiles) == 0 {
		return inspection{state: bindingNotConfigured}, nil
	}
	if len(profiles) > 1 {
		return inspection{}, errors.New("conflicting managed Stickguy Cursor hooks")
	}
	var profile string
	for value := range profiles {
		profile = value
	}
	if want, _ := managedProfile(expected("sessionStart", base).Command, "sessionStart"); profile != want {
		return inspection{state: bindingOtherProfile, previousProfile: profile}, nil
	}
	if len(present) != len(agentactivity.CursorEvents) {
		return inspection{state: bindingPartial, previousProfile: profile}, nil
	}
	return inspection{state: bindingCurrent, previousProfile: profile}, nil
}

func install(doc *document, base string) {
	removeManaged(doc)
	if doc.Hooks == nil {
		doc.Hooks = map[string][]handler{}
	}
	for _, event := range agentactivity.CursorEvents {
		doc.Hooks[event] = append(doc.Hooks[event], expected(event, base))
	}
	doc.Version = schemaVersion
}

// removeManaged drops every Stickguy hook regardless of which profile wrote it,
// leaving unrelated handlers and empty-but-member-owned events untouched. An
// event left with no handlers is deleted, because an empty array is not
// configuration the member wrote.
func removeManaged(doc *document) bool {
	changed := false
	for event, handlers := range doc.Hooks {
		kept := make([]handler, 0, len(handlers))
		for _, candidate := range handlers {
			if managed(candidate.Command) {
				changed = true
				continue
			}
			kept = append(kept, candidate)
		}
		if len(kept) == 0 {
			delete(doc.Hooks, event)
			continue
		}
		doc.Hooks[event] = kept
	}
	return changed
}

func read(path string) (*document, error) {
	doc := &document{Hooks: map[string][]handler{}, rest: map[string]json.RawMessage{}, rawHooks: map[string]json.RawMessage{}, modeled: map[string]bool{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return doc, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Cursor hooks configuration: %w", err)
	}
	if len(data) > maxConfigBytes {
		return nil, errors.New("Cursor hooks configuration exceeds 1 MiB")
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return doc, nil
	}
	if err = json.Unmarshal(data, &doc.rest); err != nil {
		return nil, fmt.Errorf("parse Cursor hooks configuration: %w", err)
	}
	if raw, ok := doc.rest["version"]; ok {
		if err = json.Unmarshal(raw, &doc.Version); err != nil {
			return nil, errors.New("Cursor hooks.json version is not a number; refusing to edit it")
		}
		delete(doc.rest, "version")
	}
	if raw, ok := doc.rest["hooks"]; ok {
		if err = json.Unmarshal(raw, &doc.rawHooks); err != nil {
			return nil, errors.New("Cursor hooks.json hooks must be a JSON object; refusing to edit it")
		}
		delete(doc.rest, "hooks")
		for event, rawHandlers := range doc.rawHooks {
			var handlers []handler
			if err = json.Unmarshal(rawHandlers, &handlers); err != nil {
				// An event whose handlers Stickguy cannot model is left exactly
				// as written and is never a place a managed hook can hide.
				continue
			}
			doc.Hooks[event] = handlers
			doc.modeled[event] = true
		}
	}
	return doc, nil
}

func write(path string, doc *document) error {
	out := map[string]json.RawMessage{}
	for key, value := range doc.rest {
		out[key] = value
	}
	version, err := json.Marshal(doc.Version)
	if err != nil {
		return err
	}
	out["version"] = version
	hooks := map[string]json.RawMessage{}
	for event, rawHandlers := range doc.rawHooks {
		if !doc.modeled[event] {
			// An event this package could not decode keeps its original bytes.
			hooks[event] = rawHandlers
		}
	}
	for event, handlers := range doc.Hooks {
		encoded, encodeErr := json.Marshal(handlers)
		if encodeErr != nil {
			return encodeErr
		}
		hooks[event] = encoded
	}
	encodedHooks, err := json.Marshal(hooks)
	if err != nil {
		return err
	}
	out["hooks"] = encodedHooks
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create Cursor configuration directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".stickguy-cursor-hooks-*")
	if err != nil {
		return fmt.Errorf("create temporary Cursor hooks configuration: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err = temporary.Chmod(0o644); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write temporary Cursor hooks configuration: %w", err)
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("activate Cursor hooks configuration: %w", err)
	}
	return nil
}

type fileSnapshot struct {
	exists bool
	data   []byte
	mode   os.FileMode
}

func capture(path string) (fileSnapshot, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fileSnapshot{}, nil
	}
	if err != nil {
		return fileSnapshot{}, err
	}
	data, err := os.ReadFile(path)
	return fileSnapshot{exists: true, data: data, mode: info.Mode().Perm()}, err
}

func restore(path string, snapshot fileSnapshot) error {
	if !snapshot.exists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, snapshot.data, snapshot.mode)
}
