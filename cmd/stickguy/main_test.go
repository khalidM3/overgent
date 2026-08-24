package main

import (
	"encoding/json"
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
