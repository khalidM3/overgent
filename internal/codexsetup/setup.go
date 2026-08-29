package codexsetup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/stickguy/stickguy/internal/codexappserver"
	"github.com/stickguy/stickguy/internal/hookconfig"
)

const (
	beginMarker = "# BEGIN STICKGUY MANAGED MCP\n"
	endMarker   = "# END STICKGUY MANAGED MCP\n"
)

type Status struct {
	Configured      bool   `json:"configured"`
	ConfigPath      string `json:"configPath"`
	HookPath        string `json:"hookPath,omitempty"`
	Hooks           string `json:"hooks"`
	Binding         string `json:"binding"`
	PreviousProfile string `json:"previousProfile,omitempty"`
	// CheckedProfile names the profile this status was evaluated against, so an
	// `other_profile` result can be read without guessing what it was compared
	// with. Without it the honest answer "Codex is bound to a different profile
	// than the one you asked about" is indistinguishable from the alarming one
	// "Codex is bound to somebody else's profile", and the difference is usually
	// just a missing --config-root.
	CheckedProfile  string `json:"checkedProfile"`
	RestartRequired bool   `json:"restartRequired"`
	// Trust reports whether Codex will actually run the installed hooks. Files
	// on disk are not enough: Codex skips an untrusted hook silently (ADR-051).
	Trust TrustReport `json:"trust"`
}

type Manager struct {
	ProjectRoot, ConfigRoot, Executable string
	Portable                            bool
	// CodexHome overrides the Codex home directory. Empty resolves $CODEX_HOME
	// and then ~/.codex, so tests never touch real contributor state.
	CodexHome string
	// CodexExecutable overrides Codex discovery for trust repair.
	CodexExecutable string
	// Version is reported to Codex during the app-server handshake.
	Version string
}

var rebindHooks = hookconfig.Rebind

// Setup installs the binding using a background context.
func (m Manager) Setup() (Status, error) { return m.SetupContext(context.Background()) }

// SetupContext installs the project MCP binding and the user-level activity
// hooks, then asks Codex to trust those hooks so they can actually run.
func (m Manager) SetupContext(ctx context.Context) (Status, error) {
	resolved, expected, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	current, err := readOptional(resolved)
	if err != nil {
		return Status{}, err
	}
	state, err := inspect(current, expected)
	if err != nil {
		return Status{}, err
	}
	if state.Binding == "other_profile" {
		return state, errors.New("Codex is connected to another Stickguy profile; explicit reconnect is required")
	}
	if !state.Configured {
		if strings.Contains(current, "[mcp_servers.stickguy]") {
			return Status{}, errors.New("unmanaged Codex stickguy MCP table already exists")
		}
		if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
			return Status{}, fmt.Errorf("create project Codex config directory: %w", err)
		}
		separator := ""
		if len(current) > 0 && !strings.HasSuffix(current, "\n") {
			separator = "\n"
		}
		if len(current) > 0 {
			separator += "\n"
		}
		if err := atomicWrite(resolved, []byte(current+separator+expected), 0o644); err != nil {
			return Status{}, err
		}
	}
	hookPath, hookCommand, err := m.hookDetails()
	if err != nil {
		return Status{}, err
	}
	if err := hookconfig.Install(hookPath, hookCommand); err != nil {
		return Status{}, fmt.Errorf("install Codex activity hooks: %w", err)
	}
	trust := inspectTrust(m, ctx, hookPath, hookCommand, true)
	return Status{
		Configured: true, ConfigPath: resolved, HookPath: hookPath, CheckedProfile: m.checkedProfile(),
		Hooks: hookState(trust), Binding: "current", RestartRequired: true, Trust: trust,
	}, nil
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

// Status reports the binding using a background context.
func (m Manager) Status() (Status, error) { return m.StatusContext(context.Background()) }

// StatusContext reports the binding without changing it. A binding whose files
// are present but whose hooks Codex has not trusted reports "needs_review", not
// "active": Codex skips an untrusted hook silently, so reporting it as working
// is what hid this failure in the first place.
func (m Manager) StatusContext(ctx context.Context) (Status, error) {
	resolved, expected, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	current, err := readOptional(resolved)
	if err != nil {
		return Status{}, err
	}
	status, err := inspect(current, expected)
	if err != nil {
		return Status{}, err
	}
	hookPath, hookCommand, err := m.hookDetails()
	if err != nil {
		return Status{}, err
	}
	hooks, err := hookconfig.Inspect(hookPath, hookCommand)
	if err != nil {
		return Status{}, err
	}
	status.ConfigPath, status.HookPath, status.CheckedProfile = resolved, hookPath, m.checkedProfile()
	switch {
	case status.Binding == "current" && hooks.State == hookconfig.BindingCurrent:
		status.Configured, status.Hooks, status.RestartRequired = true, "active", true
		status.Trust = inspectTrust(m, ctx, hookPath, hookCommand, false)
		status.Hooks = hookState(status.Trust)
	case status.Binding == "other_profile" && (hooks.State == hookconfig.BindingOtherProfile || hooks.State == hookconfig.BindingNotConfigured):
		status.Configured, status.Hooks, status.RestartRequired = false, "other_profile", true
	case status.Binding == "current" && hooks.State == hookconfig.BindingOtherProfile:
		status.Configured, status.Binding, status.Hooks, status.RestartRequired = false, "other_profile", "other_profile", true
	case status.Binding == "not_configured" && hooks.State == hookconfig.BindingOtherProfile:
		status.Binding, status.Hooks, status.RestartRequired = "other_profile", "other_profile", true
	case hooks.State == hookconfig.BindingPartial || status.Binding == "current" || hooks.State == hookconfig.BindingCurrent:
		status.Configured, status.Binding, status.Hooks, status.RestartRequired = false, "partial", "partial", true
	default:
		status.Configured, status.Binding, status.Hooks = false, "not_configured", "not_configured"
	}
	return status, nil
}

// Rebind transactionally replaces a structurally recognized Stickguy binding
// from another profile. Unknown drift remains an error. Both config files are
// restored if either managed rewrite fails.
func (m Manager) Rebind() (Status, error) {
	resolved, expected, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	current, err := readOptional(resolved)
	if err != nil {
		return Status{}, err
	}
	state, err := inspect(current, expected)
	if err != nil {
		return Status{}, err
	}
	hookPath, hookCommand, err := m.hookDetails()
	if err != nil {
		return Status{}, err
	}
	configSnapshot, err := capture(resolved)
	if err != nil {
		return Status{}, err
	}
	hookSnapshot, err := capture(hookPath)
	if err != nil {
		return Status{}, err
	}
	next := current
	if state.Binding == "other_profile" {
		start := strings.Index(current, beginMarker)
		end := strings.Index(current, endMarker) + len(endMarker)
		next = current[:start] + expected + current[end:]
	} else if state.Binding == "not_configured" {
		separator := ""
		if len(next) > 0 && !strings.HasSuffix(next, "\n") {
			separator = "\n"
		}
		if len(next) > 0 {
			separator += "\n"
		}
		next += separator + expected
	}
	if next != current {
		if err = atomicWrite(resolved, []byte(next), 0o644); err != nil {
			return Status{}, err
		}
	}
	if err = rebindHooks(hookPath, hookCommand); err != nil {
		restoreErr := restore(resolved, configSnapshot)
		if hookRestoreErr := restore(hookPath, hookSnapshot); restoreErr == nil {
			restoreErr = hookRestoreErr
		}
		if restoreErr != nil {
			return Status{}, fmt.Errorf("rebind Codex hooks: %v; rollback: %w", err, restoreErr)
		}
		return Status{}, fmt.Errorf("rebind Codex hooks: %w", err)
	}
	trust := inspectTrust(m, context.Background(), hookPath, hookCommand, true)
	return Status{
		Configured: true, ConfigPath: resolved, HookPath: hookPath, CheckedProfile: m.checkedProfile(),
		Hooks: hookState(trust), Binding: "current", RestartRequired: true, Trust: trust,
	}, nil
}

func (m Manager) Remove() (Status, error) {
	resolved, expected, err := m.resolve()
	if err != nil {
		return Status{}, err
	}
	current, err := readOptional(resolved)
	if err != nil {
		return Status{}, err
	}
	status, err := inspect(current, expected)
	if err != nil {
		return Status{}, err
	}
	if status.Configured {
		start := strings.Index(current, expected)
		remaining := current[:start] + current[start+len(expected):]
		if start >= 2 && current[start-2:start] == "\n\n" {
			remaining = current[:start-1] + current[start+len(expected):]
		}
		if err := atomicWrite(resolved, []byte(remaining), 0o644); err != nil {
			return Status{}, err
		}
	}
	hookPath, _, err := m.hookDetails()
	if err != nil {
		return Status{}, err
	}
	// Hooks live at the user layer and are shared by every registered project,
	// so removing one project must not disarm the others. RemoveHooks is the
	// deliberate teardown, called once when no project remains.
	return Status{ConfigPath: resolved, HookPath: hookPath, Hooks: "not_configured", Binding: "not_configured"}, nil
}

// RemoveHooks deletes the managed user-level Codex hooks. Call it only after
// every project binding has been removed; the hooks are shared.
func (m Manager) RemoveHooks() error {
	hookPath, hookCommand, err := m.hookDetails()
	if err != nil {
		return err
	}
	installed, err := hookconfig.Status(hookPath, hookCommand)
	if err != nil {
		return err
	}
	if !installed {
		return nil
	}
	return hookconfig.Remove(hookPath, hookCommand)
}

// hookState maps a trust report onto the status vocabulary the desktop app
// renders. "active" is reserved for hooks Codex will actually run.
func hookState(trust TrustReport) string {
	if trust.Satisfied() {
		return "active"
	}
	return "needs_review"
}

// hookDetails resolves the Codex hook file and the exact managed command.
//
// Codex hooks are installed at the user layer (`$CODEX_HOME/hooks.json`) rather
// than per project. Two reasons, both load-bearing. Trust is recorded against a
// hook's content hash, so one user-level definition needs a single review for
// every project a member registers, where per-project files need one review
// each. And Codex silently ignores a project-level `.codex/hooks.json` when the
// working directory is a Git worktree (openai/codex#27133), which is exactly
// where agents run during parallel work; user-level hooks are unaffected.
//
// The cost is that the hook fires for every repository the member opens in
// Codex. `agent-hook` already resolves the event against registered workspaces
// and exits without effect when the working directory is not one.
func (m Manager) hookDetails() (string, string, error) {
	home, err := codexappserver.Home(m.CodexHome)
	if err != nil {
		return "", "", err
	}
	var command string
	if m.Portable {
		command, err = hookconfig.PortableCommand("codex")
	} else {
		command, err = hookconfig.Command(m.Executable, m.ConfigRoot, "codex")
	}
	return filepath.Join(home, "hooks.json"), command, err
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
	info, err := os.Stat(project)
	if err != nil || !info.IsDir() {
		return "", "", errors.New("project root is not a directory")
	}
	command := "stickguy"
	args := strconv.Quote("mcp")
	if !m.Portable {
		configRoot, resolveErr := filepath.Abs(m.ConfigRoot)
		if resolveErr != nil {
			return "", "", resolveErr
		}
		executable, resolveErr := filepath.Abs(m.Executable)
		if resolveErr != nil || executable == "" {
			return "", "", errors.New("resolve Stickguy executable")
		}
		command = executable
		args = strconv.Quote("--config-root") + ", " + strconv.Quote(configRoot) + ", " + args
	}
	block := beginMarker + "[mcp_servers.stickguy]\n" +
		"command = " + strconv.Quote(command) + "\n" +
		"args = [" + args + "]\n" +
		"cwd = " + strconv.Quote(project) + "\n" +
		"required = false\nstartup_timeout_sec = 10\ntool_timeout_sec = 60\n" + endMarker
	return filepath.Join(project, ".codex", "config.toml"), block, nil
}

func inspect(current, expected string) (Status, error) {
	starts, ends := strings.Count(current, beginMarker), strings.Count(current, endMarker)
	if starts == 0 && ends == 0 {
		return Status{Binding: "not_configured"}, nil
	}
	if starts != 1 || ends != 1 {
		return Status{}, errors.New("managed Codex MCP block markers are incomplete or duplicated")
	}
	start := strings.Index(current, beginMarker)
	end := strings.Index(current, endMarker) + len(endMarker)
	if start < 0 || end < start {
		return Status{}, errors.New("managed Codex MCP block drifted; refusing to overwrite or remove it")
	}
	block := current[start:end]
	if block != expected {
		currentManaged, currentOK := parseManagedBlock(block)
		expectedManaged, expectedOK := parseManagedBlock(expected)
		if !currentOK || !expectedOK || currentManaged.cwd != expectedManaged.cwd {
			return Status{}, errors.New("managed Codex MCP block drifted; refusing to overwrite or remove it")
		}
		return Status{Binding: "other_profile", PreviousProfile: currentManaged.profile, RestartRequired: true}, nil
	}
	return Status{Configured: true, Binding: "current", RestartRequired: true}, nil
}

type managedBlock struct{ cwd, profile string }

func parseManagedBlock(block string) (managedBlock, bool) {
	lines := strings.Split(block, "\n")
	if len(lines) != 10 || lines[0] != strings.TrimSuffix(beginMarker, "\n") || lines[1] != "[mcp_servers.stickguy]" ||
		lines[5] != "required = false" || lines[6] != "startup_timeout_sec = 10" || lines[7] != "tool_timeout_sec = 60" ||
		lines[8] != strings.TrimSuffix(endMarker, "\n") || lines[9] != "" {
		return managedBlock{}, false
	}
	command, ok := unquoteField(lines[2], "command = ")
	if !ok || command != "stickguy" && !filepath.IsAbs(command) {
		return managedBlock{}, false
	}
	var args []string
	if !strings.HasPrefix(lines[3], "args = [") || !strings.HasSuffix(lines[3], "]") || json.Unmarshal([]byte(strings.TrimPrefix(lines[3], "args = ")), &args) != nil {
		return managedBlock{}, false
	}
	cwd, ok := unquoteField(lines[4], "cwd = ")
	if !ok || !filepath.IsAbs(cwd) {
		return managedBlock{}, false
	}
	if len(args) == 1 && args[0] == "mcp" && command == "stickguy" {
		return managedBlock{cwd: cwd}, true
	}
	if len(args) != 3 || args[0] != "--config-root" || !filepath.IsAbs(args[1]) || args[2] != "mcp" || !filepath.IsAbs(command) {
		return managedBlock{}, false
	}
	return managedBlock{cwd: cwd, profile: filepath.Clean(args[1])}, true
}

func unquoteField(line, prefix string) (string, bool) {
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	value, err := strconv.Unquote(strings.TrimPrefix(line, prefix))
	return value, err == nil
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

func readOptional(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read project Codex config: %w", err)
	}
	if len(data) > 1<<20 {
		return "", errors.New("project Codex config exceeds 1 MiB")
	}
	return string(data), nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".stickguy-config-*")
	if err != nil {
		return fmt.Errorf("create temporary Codex config: %w", err)
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err = temporary.Chmod(mode); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write temporary Codex config: %w", err)
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("activate Codex config: %w", err)
	}
	return nil
}
