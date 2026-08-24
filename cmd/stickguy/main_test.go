package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVersionEnvelope(t *testing.T) {
	b, err := json.Marshal(versionInfo{Version: "dev", Commit: "abc", BuildTime: "now", SchemaMinimum: 1, SchemaMaximum: 1})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["schemaMinimum"] != float64(1) || got["schemaMaximum"] != float64(1) {
		t.Fatalf("unexpected protocol range: %s", b)
	}
}

func TestDevelopmentAgentSetupIsExplicitAndUsesBuiltExecutable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	for _, agent := range []string{"codex", "claude"} {
		project, state := t.TempDir(), t.TempDir()
		if err := run([]string{"--config-root", state, "setup", agent, "--development", "--project-root", project}); err != nil {
			t.Fatalf("%s development setup: %v", agent, err)
		}
		path := filepath.Join(project, ".mcp.json")
		if agent == "codex" {
			path = filepath.Join(project, ".codex", "config.toml")
		}
		contents, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(contents), state) || !strings.Contains(string(contents), "stickguy") {
			t.Fatalf("%s development config path=%s err=%v contents=%q", agent, path, err, contents)
		}
	}
}

func TestNarrowedAgentSetupFailsClosed(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	for _, agent := range []string{"codex", "claude"} {
		err := run([]string{"--config-root", t.TempDir(), "setup", agent, "--project-root", t.TempDir()})
		if err == nil || !strings.Contains(err.Error(), "withheld") || !strings.Contains(err.Error(), "Git/manual") {
			t.Fatalf("%s setup error=%v", agent, err)
		}
	}
}
