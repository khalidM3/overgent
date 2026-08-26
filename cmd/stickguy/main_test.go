package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/stickguy/stickguy/internal/daemon"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestAgentHookEmitsExactVendorJSONOnlyWithPendingContext(t *testing.T) {
	for _, vendor := range []string{"claude", "codex"} {
		for _, event := range []string{"SessionStart", "UserPromptSubmit"} {
			root := t.TempDir()
			input := []byte(`{"session_id":"fixture-session","cwd":"` + root + `","hook_event_name":"` + event + `","prompt":"continue"}`)
			var output bytes.Buffer
			calls := 0
			call := func(_ context.Context, _ string, request daemon.Request) (daemon.Response, error) {
				calls++
				if request.Method == "agent_event" {
					return daemon.Response{OK: true}, nil
				}
				return daemon.Response{OK: true, Data: map[string]any{"additionalContext": "Coordination update: re-read backend.Refresh."}}, nil
			}
			if err := runAgentHook(context.Background(), "unused", vendor, bytes.NewReader(input), &output, call); err != nil {
				t.Fatal(err)
			}
			want := `{"hookSpecificOutput":{"hookEventName":"` + event + `","additionalContext":"Coordination update: re-read backend.Refresh."}}` + "\n"
			if output.String() != want || calls != 2 {
				t.Fatalf("%s %s output=%q calls=%d", vendor, event, output.String(), calls)
			}
		}
	}
}

func TestAgentHookPreservesEmptyObservationOutput(t *testing.T) {
	root := t.TempDir()
	input := []byte(`{"session_id":"fixture-session","cwd":"` + root + `","hook_event_name":"UserPromptSubmit","prompt":"continue"}`)
	var output bytes.Buffer
	call := func(_ context.Context, _ string, request daemon.Request) (daemon.Response, error) {
		if request.Method == "agent_event" {
			return daemon.Response{OK: true}, nil
		}
		return daemon.Response{OK: true, Data: map[string]any{}}, nil
	}
	if err := runAgentHook(context.Background(), "unused", "claude", bytes.NewReader(input), &output, call); err != nil || output.Len() != 0 {
		t.Fatalf("err=%v output=%q", err, output.String())
	}
}

func TestAgentHookLoopbackDeliversOnceAndRevisedItemAgain(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	socket := shortTestSocket(t)
	revision := 1
	delivered := map[int]bool{}
	var deliveryMu sync.Mutex
	go func() {
		_ = daemon.Serve(ctx, socket, func(_ context.Context, request daemon.Request) daemon.Response {
			if request.Method == "agent_event" {
				return daemon.Response{OK: true}
			}
			deliveryMu.Lock()
			defer deliveryMu.Unlock()
			if request.Method != "agent_injection" || delivered[revision] {
				return daemon.Response{OK: true, Data: map[string]any{}}
			}
			delivered[revision] = true
			return daemon.Response{OK: true, Data: map[string]any{"additionalContext": "Coordination update: backend.Refresh revision changed."}}
		})
	}()
	waitForSocket(t, socket)
	root := t.TempDir()
	input := []byte(`{"session_id":"fixture-session","cwd":"` + root + `","hook_event_name":"UserPromptSubmit","prompt":"continue"}`)
	invoke := func() string {
		var output bytes.Buffer
		if err := runAgentHook(context.Background(), socket, "claude", bytes.NewReader(input), &output, daemon.Call); err != nil {
			t.Fatal(err)
		}
		return output.String()
	}
	if first := invoke(); !strings.Contains(first, "backend.Refresh") {
		t.Fatalf("first output=%q", first)
	}
	if second := invoke(); second != "" {
		t.Fatalf("same revision output=%q", second)
	}
	deliveryMu.Lock()
	revision = 2
	deliveryMu.Unlock()
	if third := invoke(); !strings.Contains(third, "backend.Refresh") {
		t.Fatalf("revised output=%q", third)
	}
}

func TestAgentHookSlowServiceFailsOpenWithoutOutput(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("local service currently supports macOS")
	}
	serviceContext, cancelService := context.WithCancel(context.Background())
	defer cancelService()
	socket := shortTestSocket(t)
	go func() {
		_ = daemon.Serve(serviceContext, socket, func(context.Context, daemon.Request) daemon.Response {
			time.Sleep(500 * time.Millisecond)
			return daemon.Response{OK: true}
		})
	}()
	waitForSocket(t, socket)
	root := t.TempDir()
	input := []byte(`{"session_id":"fixture-session","cwd":"` + root + `","hook_event_name":"UserPromptSubmit","prompt":"continue"}`)
	var output bytes.Buffer
	hookContext, cancelHook := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancelHook()
	started := time.Now()
	if err := runAgentHook(hookContext, socket, "codex", bytes.NewReader(input), &output, daemon.Call); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 || time.Since(started) > 400*time.Millisecond {
		t.Fatalf("output=%q elapsed=%s", output.String(), time.Since(started))
	}
}

func waitForSocket(t *testing.T, socket string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("socket %s was not ready", socket)
}

func shortTestSocket(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("/private/tmp", "sg-hook-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return filepath.Join(directory, "service.sock")
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
