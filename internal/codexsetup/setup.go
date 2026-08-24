package codexsetup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/stickguy/stickguy/internal/hookconfig"
)

const (
	beginMarker = "# BEGIN STICKGUY MANAGED MCP\n"
	endMarker   = "# END STICKGUY MANAGED MCP\n"
)

type Status struct {
	Configured bool   `json:"configured"`
	ConfigPath string `json:"configPath"`
	Hooks      string `json:"hooks"`
}

type Manager struct {
	ProjectRoot, ConfigRoot, Executable string
	Portable                            bool
}

func (m Manager) Setup() (Status, error) {
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
	return Status{Configured: true, ConfigPath: resolved, Hooks: "active"}, nil
}

func (m Manager) Status() (Status, error) {
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
	hooks, err := hookconfig.Status(hookPath, hookCommand)
	if err != nil {
		return Status{}, err
	}
	status.Configured = status.Configured && hooks
	status.ConfigPath = resolved
	if hooks {
		status.Hooks = "active"
	} else {
		status.Hooks = "not_configured"
	}
	return status, nil
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
	return Status{ConfigPath: resolved, Hooks: "not_configured"}, nil
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
		command, err = hookconfig.PortableCommand("codex")
	} else {
		command, err = hookconfig.Command(m.Executable, m.ConfigRoot, "codex")
	}
	return filepath.Join(project, ".codex", "hooks.json"), command, err
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
		return Status{}, nil
	}
	if starts != 1 || ends != 1 {
		return Status{}, errors.New("managed Codex MCP block markers are incomplete or duplicated")
	}
	start := strings.Index(current, beginMarker)
	end := strings.Index(current, endMarker) + len(endMarker)
	if start < 0 || end < start || current[start:end] != expected {
		return Status{}, errors.New("managed Codex MCP block drifted; refusing to overwrite or remove it")
	}
	return Status{Configured: true}, nil
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
