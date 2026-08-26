package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stickguy/stickguy/internal/config"
)

func TestCapabilityConfigRequiresKnownUniqueTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capabilities.json")
	if err := os.WriteFile(path, []byte(`{"required":["structural"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	required, err := loadCapabilityConfig(path)
	if err != nil || !required[capabilityStructural] || len(required) != 1 {
		t.Fatalf("required=%v err=%v", required, err)
	}
	for _, contents := range []string{`{"required":["unknown"]}`, `{"required":["structural","structural"]}`} {
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadCapabilityConfig(path); err == nil {
			t.Fatalf("config %s must fail", contents)
		}
	}
}

func TestFutureAssertionsStayVisibleWithoutFailingRequiredGate(t *testing.T) {
	required := map[capability]bool{capabilityStructural: true}
	report := scenarioReport{Assertions: []assertionResult{
		taggedAssertion("structural", capabilityStructural, true, "observed", required),
		taggedAssertion("future", capabilityContract, false, "not landed", required),
	}}
	finalizeScenario(&report)
	if report.Status != statusPass {
		t.Fatalf("scenario status=%s", report.Status)
	}
	if report.Assertions[1].Status != statusNotYetImplemented {
		t.Fatalf("future assertion status=%s", report.Assertions[1].Status)
	}
}

func TestRequiredCapabilityFailureFailsScenario(t *testing.T) {
	required := map[capability]bool{capabilityStructural: true}
	report := scenarioReport{Assertions: []assertionResult{
		taggedAssertion("silence", capabilityStructural, false, "unexpected finding", required),
	}}
	finalizeScenario(&report)
	if report.Status != statusFail {
		t.Fatalf("scenario status=%s", report.Status)
	}
}

func TestRoutingCountsExpectedTargetsOnly(t *testing.T) {
	environment := &scenarioEnvironment{
		workspaceA: configWorkspace("wrk_a"), workspaceB: configWorkspace("wrk_b"),
	}
	expected := []expectedFinding{{Kind: "direct_collision", TargetWorkstream: "wrk_a+wrk_b"}}
	findings := []actualFinding{{Kind: "direct_collision", WorkstreamIDs: []string{"wrk_a", "wrk_b"}}, {Kind: "redundant_work", WorkstreamIDs: []string{"wrk_a"}}}
	correct, routed := routingCounts(expected, findings, environment)
	if correct != 2 || routed != 3 {
		t.Fatalf("correct=%d routed=%d", correct, routed)
	}
}

func configWorkspace(workstreamID string) config.Workspace {
	return config.Workspace{WorkstreamID: workstreamID}
}
