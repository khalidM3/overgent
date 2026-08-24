package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfficialSDKFixtureLifecycleAndBounds(t *testing.T) {
	root := t.TempDir()
	registryPath := filepath.Join(t.TempDir(), "registry.json")
	b, err := json.Marshal(registry{Workspaces: []workspace{{Root: root, ProjectID: "p", WorkspaceID: "w", WorkstreamID: "s"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath, b, 0o600); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	f := &fixture{registryPath: registryPath, seen: make(map[string]struct{})}
	for _, name := range []string{"begin_work", "check_coordination", "report_checkpoint", "finish_work"} {
		_, got, err := f.call(name, toolInput{IdempotencyKey: name + "-1", Summary: "bounded fixture"})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.Tool != name || got.Brief.BriefID != "brf_fixture_1" {
			t.Fatalf("%s: %#v", name, got)
		}
	}
	_, got, err := f.call("begin_work", toolInput{IdempotencyKey: "begin_work-1"})
	if err != nil || !got.Duplicate {
		t.Fatalf("expected duplicate, got %#v, %v", got, err)
	}
	_, _, err = f.call("report_checkpoint", toolInput{Summary: strings.Repeat("x", 241)})
	if err == nil {
		t.Fatal("expected bounded-length rejection")
	}
}

func TestSDKInstructionsFrontLoadBoundary(t *testing.T) {
	if len(instructions) < 512 {
		t.Fatalf("instructions too short: %d", len(instructions))
	}
	front := instructions[:512]
	for _, want := range []string{"advisory", "begin_work", "check_coordination", "report_checkpoint", "finish_work", "Never send source", "prompts", "transcripts", "secrets"} {
		if !strings.Contains(front, want) {
			t.Errorf("first 512 missing %q", want)
		}
	}
}
