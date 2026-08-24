package claudesetup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

type Status struct {
	Configured bool   `json:"configured"`
	ConfigPath string `json:"configPath"`
	Approval   string `json:"approval"`
}

type Manager struct {
	ProjectRoot, ConfigRoot, Executable string
	Portable                            bool
}

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
		if !reflect.DeepEqual(current, expected) {
			return Status{}, errors.New("Claude stickguy MCP entry exists but differs; refusing to overwrite it")
		}
		return status(path, true), nil
	}
	servers["stickguy"] = expected
	if err := writeJSON(path, document); err != nil {
		return Status{}, err
	}
	return status(path, true), nil
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
	if exists && !reflect.DeepEqual(current, expected) {
		return Status{}, errors.New("Claude stickguy MCP entry drifted")
	}
	return status(path, exists), nil
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
		return status(path, false), nil
	}
	if !reflect.DeepEqual(current, expected) {
		return Status{}, errors.New("Claude stickguy MCP entry drifted; refusing to remove it")
	}
	delete(servers, "stickguy")
	if err := writeJSON(path, document); err != nil {
		return Status{}, err
	}
	return status(path, false), nil
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
	return Status{Configured: configured, ConfigPath: path, Approval: "required_by_claude"}
}
