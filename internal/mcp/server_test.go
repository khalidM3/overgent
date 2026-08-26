//go:build darwin

package mcp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
)

func TestOfficialSDKListsAndCallsAllLifecycleTools(t *testing.T) {
	root, err := os.MkdirTemp("/private/tmp", "stickguy-mcp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	paths, err := config.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- daemon.Serve(ctx, paths.Socket, func(_ context.Context, request daemon.Request) daemon.Response {
			return daemon.Response{OK: true, Data: map[string]any{"duplicate": false, "intentRevision": 1, "degraded": true, "degradation": "fixture"}}
		})
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(paths.Socket); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	bridge := &server{paths: paths, cfg: config.Config{Workspaces: []config.Workspace{{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", Root: filepath.Dir(root)}}}}
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := newSDK(bridge).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "stickguy-conformance", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	want := []string{"acknowledge_context", "begin_work", "check_coordination", "finish_work", "get_resolutions", "report_checkpoint", "report_event", "update_intent"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tools=%v", names)
	}
	calls := map[string]map[string]any{
		"begin_work":          {"workspace_id": "wsp_fixture", "idempotency_key": "begin-1", "title": "Bounded work", "outcome": "Prove lifecycle"},
		"update_intent":       {"workspace_id": "wsp_fixture", "idempotency_key": "intent-2", "revision": 1, "title": "Bounded work", "outcome": "Refine lifecycle"},
		"check_coordination":  {"workspace_id": "wsp_fixture", "trigger": "before_broad_edit", "approximate_token_budget": 400},
		"report_checkpoint":   {"workspace_id": "wsp_fixture", "checkpoint_id": "chk_fixture_1", "summary": "Behavior is verified"},
		"acknowledge_context": {"workspace_id": "wsp_fixture", "brief_id": "brf_fixture", "considered_item_ids": []string{"itm_fixture"}},
		"finish_work":         {"workspace_id": "wsp_fixture", "idempotency_key": "finish-1", "outcome": "Lifecycle complete", "summary": "All bounded checks passed"},
		"report_event":        {"workspace_id": "wsp_fixture", "idempotency_key": "event-1", "kind": "decision", "summary": "Use MCP-only fallback"},
		"get_resolutions":     {"workspace_id": "wsp_fixture"},
	}
	for _, name := range want {
		result, err := clientSession.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: calls[name]})
		if err != nil || result.IsError {
			if result != nil {
				for _, content := range result.Content {
					if text, ok := content.(*sdkmcp.TextContent); ok {
						t.Logf("%s", text.Text)
					}
				}
			}
			t.Fatalf("%s: result=%#v err=%v", name, result, err)
		}
	}
	front := instructions[:min(512, len(instructions))]
	for _, phrase := range []string{"advisory", "begin_work", "check_coordination", "Never send source", "secrets", "separately consented"} {
		if !strings.Contains(front, phrase) {
			t.Errorf("first 512 characters omit %q", phrase)
		}
	}
	if err := clientSession.Close(); err != nil {
		t.Fatal(err)
	}
	if err := serverSession.Close(); err != nil {
		t.Fatal(err)
	}
	if response, err := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "health"}); err != nil || !response.OK {
		t.Fatalf("MCP exit stopped local service: response=%#v err=%v", response, err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func TestWorkspaceResolutionNeverGuesses(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	bridge := &server{cfg: config.Config{Workspaces: []config.Workspace{
		{ID: "wsp_a", Root: root},
		{ID: "wsp_b", Root: root},
	}}}
	if _, err = bridge.resolveWorkspace(""); err == nil || err.Error() != "workspace_registration_ambiguous" {
		t.Fatalf("ambiguous resolution err=%v", err)
	}
	if workspace, err := bridge.resolveWorkspace("wsp_b"); err != nil || workspace.ID != "wsp_b" {
		t.Fatalf("explicit resolution workspace=%#v err=%v", workspace, err)
	}
	bridge.cfg.Workspaces = nil
	if _, err = bridge.resolveWorkspace(""); err == nil || err.Error() != "workspace_not_registered" {
		t.Fatalf("missing resolution err=%v", err)
	}
}

func TestProductionBinaryStdioTransport(t *testing.T) {
	binary := os.Getenv("STICKGUY_BINARY")
	if binary == "" {
		t.Skip("set STICKGUY_BINARY to exercise the built stdio command")
	}
	root, err := os.MkdirTemp("/private/tmp", "stickguy-stdio-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	paths, err := config.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	workspaceRoot := filepath.Join(root, "project")
	if err = os.Mkdir(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = config.Save(paths, config.Config{Version: 1, Workspaces: []config.Workspace{{ID: "wsp_fixture", ProjectID: "prj_fixture", WorkstreamID: "wrk_fixture", Root: workspaceRoot}}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- daemon.Serve(ctx, paths.Socket, func(_ context.Context, _ daemon.Request) daemon.Response {
			return daemon.Response{OK: true, Data: map[string]any{"intentRevision": 1, "degraded": true, "degradation": "fixture"}}
		})
	}()
	waitSocket(t, paths.Socket)
	command := exec.Command(binary, "--config-root", root, "mcp")
	command.Dir = workspaceRoot
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "stdio-conformance", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := session.ListTools(ctx, nil)
	if err != nil || len(listed.Tools) != 7 {
		t.Fatalf("stdio tools=%d err=%v", len(listed.Tools), err)
	}
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "begin_work", Arguments: map[string]any{"workspace_id": "wsp_fixture", "idempotency_key": "stdio_1", "title": "Stdio conformance", "outcome": "Prove production transport"}})
	if err != nil || result.IsError {
		t.Fatalf("stdio call result=%#v err=%v", result, err)
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
	if response, err := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "health"}); err != nil || !response.OK {
		t.Fatalf("stdio process exit stopped service: response=%#v err=%v", response, err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("daemon did not stop")
	}
}

func waitSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("socket did not appear")
}
