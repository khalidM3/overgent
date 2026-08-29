package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSelectScenariosKeepsDeclaredOrder(t *testing.T) {
	selected, err := selectScenarios("c,a")
	if err != nil {
		t.Fatalf("select scenarios: %v", err)
	}
	if len(selected) != 2 || selected[0].ID != "A" || selected[1].ID != "C" {
		t.Fatalf("expected A then C in declared order, got %+v", selected)
	}
}

func TestSelectScenariosDefaultsToTheWholeGate(t *testing.T) {
	selected, err := selectScenarios("  ")
	if err != nil {
		t.Fatalf("select scenarios: %v", err)
	}
	if len(selected) != len(scenarioDefinitions) {
		t.Fatalf("expected the full gate, got %d of %d", len(selected), len(scenarioDefinitions))
	}
}

func TestSelectScenariosRejectsAnUnknownID(t *testing.T) {
	if _, err := selectScenarios("A,Z"); err == nil {
		t.Fatal("expected an unknown scenario to be rejected rather than silently skipped")
	}
}

func TestWrapReasonNeverSplitsAWord(t *testing.T) {
	lines := wrapReason("backend/refresh.go: Refresh changed after this session read it", 20)
	for _, line := range lines {
		if len(line) > 20 && !strings.Contains(line, " ") {
			t.Fatalf("wrapped line exceeds the width without a break point: %q", line)
		}
	}
	if strings.Join(lines, " ") != "backend/refresh.go: Refresh changed after this session read it" {
		t.Fatalf("wrapping lost or reordered words: %v", lines)
	}
}
