package main

import (
	"testing"

	"github.com/khalidM3/overgent/internal/agentactivity"
	"github.com/khalidM3/overgent/internal/config"
)

func TestReaderWorkstreamUsesPublishedEnrollmentScope(t *testing.T) {
	environment := &scenarioEnvironment{
		workspaceA: config.Workspace{
			ID:        "wsp_fixture",
			ProjectID: "prj_fixture",
			Root:      t.TempDir(),
		},
	}

	got := environment.readerWorkstream()
	local, _, ok := agentactivity.WorkstreamIDFor("claude", environment.readerSessionID)
	if !ok {
		t.Fatal("generated reader session did not produce a local workstream identity")
	}
	want := agentactivity.PublishedWorkstreamID(local, environment.workspaceA.ProjectID, environment.workspaceA.ID)
	if got != want {
		t.Fatalf("reader workstream = %q, want published identity %q", got, want)
	}
	if got == local {
		t.Fatal("reader workstream leaked the unscoped local session identity")
	}
}
