package claudesetup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/stickguy/stickguy/internal/hookconfig"
)

type Status struct {
	Configured      bool   `json:"configured"`
	ConfigPath      string `json:"configPath"`
	Approval        string `json:"approval"`
	Binding         string `json:"binding"`
	PreviousProfile string `json:"previousProfile,omitempty"`
	// CheckedProfile names the profile this status was evaluated against, so an
	// `other_profile` result can be read without guessing what it was compared
	// with. Without it the honest answer "Claude is bound to a different profile
	// than the one you asked about" is indistinguishable from the alarming one
	// "Claude is bound to somebody else's profile", and the difference is
	// usually just a missing --config-root.
	CheckedProfile  string `json:"checkedProfile"`
	RestartRequired bool   `json:"restartRequired"`
}

type Manager struct {
	ProjectRoot, ConfigRoot, Executable string
	Portable                            bool
}

var rebindHooks = hookconfig.Rebind

func (m Manager) Setup() (Status, error) {
	path, expected, document, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	servers, err := serverMap(document)
	if err != nil {
		return Status{}, err
	}
	if current, exists := servers["stickguy"]; exists {
		binding, _, bindingErr := classifyServer(current, expected)
		if bindingErr != nil {
			return Status{}, bindingErr
		}
		if binding == "other_profile" {
			return Status{Binding: binding, CheckedProfile: m.checkedProfile()}, errors.New("Claude Code is connected to another Stickguy profile; explicit reconnect is required")
		}
	} else {
		servers["stickguy"] = expected
		if err := writeJSON(path, document); err != nil {
			return Status{}, err
		}
	}
	hookPath, hookCommand, err := m.hookDetails()
	if err != nil {
		return Status{}, err
	}
	if err := hookconfig.Install(hookPath, hookCommand); err != nil {
		return Status{}, fmt.Errorf("install Claude activity hooks: %w", err)
	}
	// Pre-approve only Stickguy's own coordination tools, so the harness does
	// not spend four approval prompts before the member does any work.
	if err := hookconfig.AllowTools(hookPath, hookconfig.StickguyMCPTools); err != nil {
		return Status{}, fmt.Errorf("pre-approve Stickguy coordination tools: %w", err)
	}
	result := status(path, true)
	result.CheckedProfile = m.checkedProfile()
	result.RestartRequired = true
	return result, nil
}

func (m Manager) Status() (Status, error) {
	path, expected, document, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	servers, err := serverMap(document)
	if err != nil {
		return Status{}, err
	}
	current, exists := servers["stickguy"]
	binding, previous := "not_configured", ""
	if exists {
		binding, previous, err = classifyServer(current, expected)
		if err != nil {
			return Status{}, err
		}
	}
	hookPath, hookCommand, err := m.hookDetails()
	if err != nil {
		return Status{}, err
	}
	hooks, err := hookconfig.Inspect(hookPath, hookCommand)
	if err != nil {
		return Status{}, err
	}
	result := status(path, false)
	result.CheckedProfile = m.checkedProfile()
	result.Binding, result.PreviousProfile, result.RestartRequired = binding, previous, binding != "not_configured" || hooks.State != hookconfig.BindingNotConfigured
	switch {
	case binding == "current" && hooks.State == hookconfig.BindingCurrent:
		result.Configured, result.Binding = true, "current"
	case binding == "other_profile" && (hooks.State == hookconfig.BindingOtherProfile || hooks.State == hookconfig.BindingNotConfigured):
		result.Binding = "other_profile"
	case binding == "current" && hooks.State == hookconfig.BindingOtherProfile:
		result.Binding = "other_profile"
	case binding == "not_configured" && hooks.State == hookconfig.BindingOtherProfile:
		result.Binding = "other_profile"
	case hooks.State == hookconfig.BindingPartial || binding == "current" || hooks.State == hookconfig.BindingCurrent:
		result.Binding = "partial"
	default:
		result.Binding = "not_configured"
	}
	return result, nil
}

// Rebind transactionally moves only Stickguy's recognized MCP entry and hooks
// from another profile to this one. Unrelated Claude configuration is retained.
func (m Manager) Rebind() (Status, error) {
	path, expected, document, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	servers, err := serverMap(document)
	if err != nil {
		return Status{}, err
	}
	if current, exists := servers["stickguy"]; exists {
		if _, _, err = classifyServer(current, expected); err != nil {
			return Status{}, err
		}
	}
	hookPath, hookCommand, err := m.hookDetails()
	if err != nil {
		return Status{}, err
	}
	configSnapshot, err := capture(path)
	if err != nil {
		return Status{}, err
	}
	hookSnapshot, err := capture(hookPath)
	if err != nil {
		return Status{}, err
	}
	servers["stickguy"] = expected
	if err = writeJSON(path, document); err != nil {
		return Status{}, err
	}
	if err = rebindHooks(hookPath, hookCommand); err != nil {
		restoreErr := restore(path, configSnapshot)
		if hookRestoreErr := restore(hookPath, hookSnapshot); restoreErr == nil {
			restoreErr = hookRestoreErr
		}
		if restoreErr != nil {
			return Status{}, fmt.Errorf("rebind Claude hooks: %v; rollback: %w", err, restoreErr)
		}
		return Status{}, fmt.Errorf("rebind Claude hooks: %w", err)
	}
	result := status(path, true)
	result.CheckedProfile = m.checkedProfile()
	result.RestartRequired = true
	return result, nil
}

func (m Manager) Remove() (Status, error) {
	path, expected, document, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	servers, err := serverMap(document)
	if err != nil {
		return Status{}, err
	}
	current, exists := servers["stickguy"]
	if !exists {
		// Continue so a hooks-only partial install can still be removed safely.
	} else {
		if !reflect.DeepEqual(current, expected) {
			return Status{}, errors.New("Claude stickguy MCP entry drifted; refusing to remove it")
		}
		delete(servers, "stickguy")
		if err := writeJSON(path, document); err != nil {
			return Status{}, err
		}
	}
	hookPath, hookCommand, err := m.hookDetails()
	if err != nil {
		return Status{}, err
	}
	if hooks, hookErr := hookconfig.Status(hookPath, hookCommand); hookErr != nil {
		return Status{}, hookErr
	} else if hooks {
		if hookErr = hookconfig.Remove(hookPath, hookCommand); hookErr != nil {
			return Status{}, hookErr
		}
	}
	// Withdraw exactly what Setup granted. A teardown that leaves permissions
	// behind for a server that is gone is a permission the member never revisits.
	if err = hookconfig.DisallowTools(hookPath, hookconfig.StickguyMCPTools); err != nil {
		return Status{}, fmt.Errorf("withdraw Stickguy coordination tool approval: %w", err)
	}
	result := status(path, false)
	result.CheckedProfile = m.checkedProfile()
	return result, nil
}

func (m Manager) hookDetails() (string, string, error) {
	project, err := filepath.Abs(m.ProjectRoot)
	if err != nil {
		return "", "", err
	}
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		return "", "", err
	}
	var command string
	if m.Portable {
		command, err = hookconfig.PortableCommand("claude")
	} else {
		command, err = hookconfig.Command(m.Executable, m.ConfigRoot, "claude")
	}
	return filepath.Join(project, ".claude", "settings.local.json"), command, err
}

func (m Manager) resolve() (string, map[string]any, map[string]any, error) {
	project, err := filepath.Abs(m.ProjectRoot)
	if err != nil {
		return "", nil, nil, fmt.Errorf("resolve project root: %w", err)
	}
	project, err = filepath.EvalSymlinks(project)
	if err != nil {
		return "", nil, nil, fmt.Errorf("canonicalize project root: %w", err)
	}
	if info, statErr := os.Stat(project); statErr != nil || !info.IsDir() {
		return "", nil, nil, errors.New("project root is not a directory")
	}
	path := filepath.Join(project, ".mcp.json")
	document := map[string]any{}
	data, readErr := os.ReadFile(path)
	if readErr == nil {
		if len(data) > 1<<20 {
			return "", nil, nil, errors.New("project Claude MCP config exceeds 1 MiB")
		}
		if err = json.Unmarshal(data, &document); err != nil {
			return "", nil, nil, fmt.Errorf("parse project Claude MCP config: %w", err)
		}
	} else if !os.IsNotExist(readErr) {
		return "", nil, nil, fmt.Errorf("read project Claude MCP config: %w", readErr)
	}
	command := "stickguy"
	args := []any{"mcp"}
	if !m.Portable {
		configRoot, resolveErr := filepath.Abs(m.ConfigRoot)
		if resolveErr != nil {
			return "", nil, nil, fmt.Errorf("resolve config root: %w", resolveErr)
		}
		executable, resolveErr := filepath.Abs(m.Executable)
		if resolveErr != nil || executable == "" {
			return "", nil, nil, errors.New("resolve Stickguy executable")
		}
		command, args = executable, []any{"--config-root", configRoot, "mcp"}
	}
	expected := map[string]any{"type": "stdio", "command": command, "args": args}
	return path, expected, document, nil
}

func serverMap(document map[string]any) (map[string]any, error) {
	value, exists := document["mcpServers"]
	if !exists {
		servers := map[string]any{}
		document["mcpServers"] = servers
		return servers, nil
	}
	servers, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("Claude mcpServers must be a JSON object")
	}
	return servers, nil
}

func writeJSON(path string, document map[string]any) error {
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".stickguy-mcp-*")
	if err != nil {
		return fmt.Errorf("create temporary Claude MCP config: %w", err)
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
		return fmt.Errorf("write temporary Claude MCP config: %w", err)
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("activate Claude MCP config: %w", err)
	}
	return nil
}

func status(path string, configured bool) Status {
	binding := "not_configured"
	if configured {
		binding = "current"
	}
	return Status{Configured: configured, ConfigPath: path, Approval: "required_by_claude", Binding: binding}
}

// checkedProfile names the profile a status or setup call is evaluated against.
// A portable install relies on PATH and the default profile and has no path to
// report, so it says so rather than naming a directory it does not use.
func (m Manager) checkedProfile() string {
	if m.Portable {
		return "portable"
	}
	if absolute, err := filepath.Abs(m.ConfigRoot); err == nil {
		return absolute
	}
	return m.ConfigRoot
}

func classifyServer(current, expected any) (string, string, error) {
	if reflect.DeepEqual(current, expected) {
		return "current", "", nil
	}
	entry, ok := current.(map[string]any)
	if !ok || len(entry) != 3 || entry["type"] != "stdio" {
		return "", "", errors.New("Claude stickguy MCP entry drifted; refusing to overwrite it")
	}
	command, ok := entry["command"].(string)
	if !ok || command != "stickguy" && !filepath.IsAbs(command) {
		return "", "", errors.New("Claude stickguy MCP entry drifted; refusing to overwrite it")
	}
	rawArgs, ok := entry["args"].([]any)
	if !ok {
		return "", "", errors.New("Claude stickguy MCP entry drifted; refusing to overwrite it")
	}
	args := make([]string, len(rawArgs))
	for index, value := range rawArgs {
		text, valid := value.(string)
		if !valid {
			return "", "", errors.New("Claude stickguy MCP entry drifted; refusing to overwrite it")
		}
		args[index] = text
	}
	if len(args) == 1 && args[0] == "mcp" && command == "stickguy" {
		return "other_profile", "", nil
	}
	if len(args) != 3 || args[0] != "--config-root" || !filepath.IsAbs(args[1]) || args[2] != "mcp" || !filepath.IsAbs(command) {
		return "", "", errors.New("Claude stickguy MCP entry drifted; refusing to overwrite it")
	}
	return "other_profile", filepath.Clean(args[1]), nil
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
