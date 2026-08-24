package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Workspace struct {
	ID           string `json:"id"`
	ProjectID    string `json:"projectId"`
	WorkstreamID string `json:"workstreamId"`
	Root         string `json:"root"`
	Baseline     string `json:"baseline"`
	Fingerprint  string `json:"repositoryFingerprint"`
	MemberID     string `json:"memberId"`
	SessionID    string `json:"sessionId"`
}

type Config struct {
	Version    int         `json:"version"`
	DeviceID   string      `json:"deviceId"`
	Workspaces []Workspace `json:"workspaces"`
}

type Paths struct{ Root, Config, DB, Lock, Socket string }

func DefaultRoot() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("unsupported platform %s: local service validated only on macOS", runtime.GOOS)
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(d, "Stickguy"), nil
}

func Resolve(root string) (Paths, error) {
	if runtime.GOOS != "darwin" {
		return Paths{}, fmt.Errorf("unsupported platform %s: local service validated only on macOS", runtime.GOOS)
	}
	if root == "" {
		return Paths{}, fmt.Errorf("config root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve config root: %w", err)
	}
	return Paths{Root: abs, Config: filepath.Join(abs, "config.json"), DB: filepath.Join(abs, "state.db"), Lock: filepath.Join(abs, "service.lock"), Socket: filepath.Join(abs, "service.sock")}, nil
}

func Load(paths Paths) (Config, error) {
	b, err := os.ReadFile(paths.Config)
	if os.IsNotExist(err) {
		return Config{Version: 1}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if c.Version != 1 {
		return Config{}, fmt.Errorf("unsupported config version %d", c.Version)
	}
	return c, nil
}

func Save(paths Paths, c Config) error {
	if err := os.MkdirAll(paths.Root, 0o700); err != nil {
		return fmt.Errorf("create config root: %w", err)
	}
	if err := os.Chmod(paths.Root, 0o700); err != nil {
		return fmt.Errorf("secure config root: %w", err)
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	tmp := paths.Config + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, paths.Config); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	if err := os.Chmod(paths.Config, 0o600); err != nil {
		return fmt.Errorf("secure config: %w", err)
	}
	return nil
}
