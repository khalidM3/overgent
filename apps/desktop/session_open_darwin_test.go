//go:build darwin

package main

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khalidM3/overgent/internal/agentactivity"
	"github.com/khalidM3/overgent/internal/config"
)

func sessionOpenFixture(t *testing.T, vendor, vendorSessionID string) (*OnboardingService, string, string) {
	t.Helper()
	home, stateRoot, repository := t.TempDir(), t.TempDir(), t.TempDir()
	repository, _ = filepath.EvalSymlinks(repository)
	paths, err := config.Resolve(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = config.Save(paths, config.Config{Version: 1, Workspaces: []config.Workspace{{ID: "wsp_test", ProjectID: "prj_test", WorkstreamID: "wrk_test", Root: repository}}}); err != nil {
		t.Fatal(err)
	}
	var record string
	if vendor == "codex" {
		record = filepath.Join(home, ".codex", "sessions", "2026", "08", "30", "rollout-"+vendorSessionID+".jsonl")
	} else {
		record = filepath.Join(home, ".claude", "projects", "fixture", vendorSessionID+".jsonl")
	}
	if err = os.MkdirAll(filepath.Dir(record), 0o700); err != nil {
		t.Fatal(err)
	}
	var line string
	if vendor == "codex" {
		line = `{"type":"session_meta","payload":{"id":"` + vendorSessionID + `","session_id":"` + vendorSessionID + `","cwd":"` + repository + `"}}` + "\n"
	} else {
		line = `{"type":"user","sessionId":"` + vendorSessionID + `","cwd":"` + repository + `","message":{"role":"user","content":"fixture"}}` + "\n"
	}
	if err = os.WriteFile(record, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	workstreamID, _, ok := agentactivity.WorkstreamIDFor(vendor, vendorSessionID)
	if !ok {
		t.Fatal("fixture session did not derive")
	}
	return &OnboardingService{configRoot: stateRoot, homeDirectory: home}, workstreamID, repository
}

func TestOpenClaudeSessionUsesFreshPromptURLAndReportsMissingHandler(t *testing.T) {
	service, workstreamID, repository := sessionOpenFixture(t, "claude", "b4f019ed-0c2a-4f0e-9a1d-2f7b4c1d8e55")
	var opened string
	service.openSessionURL = func(value string) error { opened = value; return nil }
	result, err := service.OpenOwningSession(workstreamID, "Review the collision before editing.", "vendor")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Opened || result.Vendor != "claude" || !strings.Contains(result.Detail, "existing session was not resumed") {
		t.Fatalf("unexpected result: %#v", result)
	}
	parsed, err := url.Parse(opened)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "claude-cli" || parsed.Host != "open" || parsed.Query().Get("cwd") != repository || parsed.Query().Get("q") != "Review the collision before editing." {
		t.Fatalf("unexpected Claude URL: %q", opened)
	}

	service.openSessionURL = func(string) error { return errors.New("no handler") }
	result, err = service.OpenOwningSession(workstreamID, "Review the collision before editing.", "vendor")
	if err != nil {
		t.Fatal(err)
	}
	if result.Opened || result.FallbackCommand == "" || !strings.Contains(result.Detail, "not registered") {
		t.Fatalf("handler absence was not visible: %#v", result)
	}
}

func TestOpenCodexSessionUsesExactContinueArgumentArray(t *testing.T) {
	const vendorSessionID = "019f54c4-fd53-7d71-a61b-9b552fc3f730"
	service, workstreamID, repository := sessionOpenFixture(t, "codex", vendorSessionID)
	bin := t.TempDir()
	codex := filepath.Join(bin, "codex")
	if err := os.WriteFile(codex, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	var executable, cwd string
	var arguments []string
	service.startSessionCommand = func(gotExecutable string, gotArguments []string, gotCWD string) error {
		executable, arguments, cwd = gotExecutable, append([]string{}, gotArguments...), gotCWD
		return nil
	}
	result, err := service.OpenOwningSession(workstreamID, "Review the finding.", "vendor")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Opened || executable != codex || cwd != repository || len(arguments) != 2 || arguments[0] != "continue" || arguments[1] != vendorSessionID {
		t.Fatalf("Codex did not receive the exact continuation arguments: result=%#v executable=%q args=%q cwd=%q", result, executable, arguments, cwd)
	}
	if strings.Contains(strings.Join(arguments, " "), "resume") {
		t.Fatal("Codex picker command was used")
	}
}

func TestOwningSessionResolutionRejectsRepositoryEscape(t *testing.T) {
	service, workstreamID, _ := sessionOpenFixture(t, "claude", "b4f019ed-0c2a-4f0e-9a1d-2f7b4c1d8e55")
	// Replace the local record with a valid identity pointing outside every
	// registered workspace. A raw vendor id alone is never enough authority.
	record := filepath.Join(service.homeDirectory, ".claude", "projects", "fixture", "b4f019ed-0c2a-4f0e-9a1d-2f7b4c1d8e55.jsonl")
	line := `{"type":"user","sessionId":"b4f019ed-0c2a-4f0e-9a1d-2f7b4c1d8e55","cwd":"` + t.TempDir() + `"}` + "\n"
	if err := os.WriteFile(record, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenOwningSession(workstreamID, "Review the finding.", "vendor"); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("escaped repository session was accepted: %v", err)
	}
}
