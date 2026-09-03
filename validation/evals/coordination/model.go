package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

type capability string

const (
	capabilityStructural capability = "structural"
	capabilityContract   capability = "contract"
	capabilityInjection  capability = "injection"
	capabilitySemantic   capability = "semantic"
	capabilityDependency capability = "dependency"
)

var allCapabilities = []capability{
	capabilityStructural,
	capabilityContract,
	capabilityInjection,
	capabilitySemantic,
	capabilityDependency,
}

type resultStatus string

const (
	statusPass              resultStatus = "pass"
	statusFail              resultStatus = "fail"
	statusNotYetImplemented resultStatus = "not_yet_implemented"
)

type capabilityConfig struct {
	Required []capability `json:"required"`
}

func loadCapabilityConfig(path string) (map[capability]bool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read capability config: %w", err)
	}
	var config capabilityConfig
	if err := json.Unmarshal(contents, &config); err != nil {
		return nil, fmt.Errorf("decode capability config: %w", err)
	}
	known := map[capability]bool{}
	for _, value := range allCapabilities {
		known[value] = true
	}
	required := map[capability]bool{}
	for _, value := range config.Required {
		if !known[value] {
			return nil, fmt.Errorf("unknown required capability %q", value)
		}
		if required[value] {
			return nil, fmt.Errorf("duplicate required capability %q", value)
		}
		required[value] = true
	}
	return required, nil
}

type expectedFinding struct {
	Kind             string     `json:"kind"`
	TargetWorkstream string     `json:"targetWorkstream"`
	Capability       capability `json:"capability"`
	NamedEvidence    string     `json:"namedEvidence,omitempty"`
}

type actualFinding struct {
	ID              string           `json:"id"`
	Kind            string           `json:"kind"`
	Severity        string           `json:"severity"`
	ConfidenceBand  string           `json:"confidenceBand"`
	WorkstreamIDs   []string         `json:"workstreamIds"`
	Evidence        []map[string]any `json:"evidence"`
	Reason          string           `json:"reason"`
	State           string           `json:"state"`
	Revision        int              `json:"revision"`
	AdvisoryActions []string         `json:"advisoryActions,omitempty"`
}

type assertionResult struct {
	Name       string       `json:"name"`
	Capability capability   `json:"capability"`
	Status     resultStatus `json:"status"`
	Observed   bool         `json:"observed"`
	Detail     string       `json:"detail"`
}

type scenarioMetrics struct {
	CorrectTargetRate   float64 `json:"correctTargetRate"`
	CorrectlyRouted     int     `json:"correctlyRouted"`
	AllRouted           int     `json:"allRouted"`
	FalseInterruptCount int     `json:"falseInterruptCount"`
	SilenceHonored      bool    `json:"silenceHonored"`
	ContextSufficient   bool    `json:"contextSufficient"`
	AdjustmentProbe     bool    `json:"adjustmentProbe"`
	// DeliveryMillis is how long the affected session waited, across turn
	// boundaries, before the correction was injected. Hosted evaluation is
	// asynchronous and its tail is the number this product is judged on, so it
	// is measured rather than hidden behind a generous timeout.
	DeliveryMillis int64 `json:"deliveryMillis"`
	WallTimeMillis int64 `json:"wallTimeMillis"`
}

type scenarioReport struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Status           resultStatus      `json:"status"`
	ExpectedRouting  string            `json:"expectedRouting"`
	ActualRouting    string            `json:"actualRouting"`
	ExpectedFindings []expectedFinding `json:"expectedFindings"`
	ActualFindings   []actualFinding   `json:"actualFindings"`
	Assertions       []assertionResult `json:"assertions"`
	Metrics          scenarioMetrics   `json:"metrics"`
	ExecutionError   string            `json:"executionError,omitempty"`
}

type aggregateMetrics struct {
	Precision          float64                     `json:"precision"`
	CorrectlyRouted    int                         `json:"correctlyRouted"`
	AllRouted          int                         `json:"allRouted"`
	FalseInterrupts    int                         `json:"falseInterrupts"`
	CapabilityStatuses map[capability]resultStatus `json:"capabilityStatuses"`
	WallTimeMillis     int64                       `json:"wallTimeMillis"`
}

type evaluationReport struct {
	SchemaVersion        string           `json:"schemaVersion"`
	GeneratedAt          string           `json:"generatedAt"`
	RequiredCapabilities []capability     `json:"requiredCapabilities"`
	Scenarios            []scenarioReport `json:"scenarios"`
	Aggregate            aggregateMetrics `json:"aggregate"`
}

func taggedAssertion(name string, tag capability, observed bool, detail string, required map[capability]bool) assertionResult {
	status := statusNotYetImplemented
	if required[tag] {
		status = statusFail
		if observed {
			status = statusPass
		}
	}
	return assertionResult{Name: name, Capability: tag, Status: status, Observed: observed, Detail: detail}
}

func finalizeScenario(report *scenarioReport) {
	if report.ExecutionError != "" {
		report.Status = statusFail
		return
	}
	hasRequired := false
	for _, assertion := range report.Assertions {
		switch assertion.Status {
		case statusFail:
			report.Status = statusFail
			return
		case statusPass:
			hasRequired = true
		}
	}
	if hasRequired {
		report.Status = statusPass
	} else {
		report.Status = statusNotYetImplemented
	}
}

func aggregateReport(report *evaluationReport, started time.Time) {
	report.Aggregate = aggregateMetrics{CapabilityStatuses: map[capability]resultStatus{}, WallTimeMillis: time.Since(started).Milliseconds()}
	for _, value := range allCapabilities {
		report.Aggregate.CapabilityStatuses[value] = statusNotYetImplemented
	}
	for _, scenario := range report.Scenarios {
		report.Aggregate.CorrectlyRouted += scenario.Metrics.CorrectlyRouted
		report.Aggregate.AllRouted += scenario.Metrics.AllRouted
		report.Aggregate.FalseInterrupts += scenario.Metrics.FalseInterruptCount
		for _, assertion := range scenario.Assertions {
			current := report.Aggregate.CapabilityStatuses[assertion.Capability]
			if assertion.Status == statusFail || current != statusFail && assertion.Status == statusPass {
				report.Aggregate.CapabilityStatuses[assertion.Capability] = assertion.Status
			}
		}
	}
	if report.Aggregate.AllRouted == 0 {
		report.Aggregate.Precision = 1
	} else {
		report.Aggregate.Precision = float64(report.Aggregate.CorrectlyRouted) / float64(report.Aggregate.AllRouted)
	}
}

func reportFailed(report evaluationReport) bool {
	for _, scenario := range report.Scenarios {
		if scenario.Status == statusFail {
			return true
		}
	}
	return false
}

func printTable(report evaluationReport) {
	fmt.Println("Overgent coordination evaluation")
	fmt.Println("SCENARIO  STATUS                   ROUTING       TARGET  FALSE-INT  WALL")
	for _, scenario := range report.Scenarios {
		fmt.Printf("%-8s  %-23s  %-12s  %5.0f%%  %9d  %4dms\n",
			scenario.ID, scenario.Status, scenario.ActualRouting,
			scenario.Metrics.CorrectTargetRate*100, scenario.Metrics.FalseInterruptCount,
			scenario.Metrics.WallTimeMillis)
		statuses := map[capability]resultStatus{}
		for _, assertion := range scenario.Assertions {
			current := statuses[assertion.Capability]
			if assertion.Status == statusFail || current != statusFail && assertion.Status == statusPass || current == "" {
				statuses[assertion.Capability] = assertion.Status
			}
		}
		var parts []string
		for _, value := range allCapabilities {
			if status, ok := statuses[value]; ok {
				parts = append(parts, fmt.Sprintf("%s=%s", value, status))
			}
		}
		fmt.Printf("          %s\n", strings.Join(parts, "  "))
		if scenario.ExecutionError != "" {
			fmt.Printf("          execution_error=%s\n", scenario.ExecutionError)
		}
	}
	fmt.Printf("aggregate precision=%.3f correctly_routed=%d all_routed=%d false_interrupts=%d wall=%dms\n",
		report.Aggregate.Precision, report.Aggregate.CorrectlyRouted, report.Aggregate.AllRouted,
		report.Aggregate.FalseInterrupts, report.Aggregate.WallTimeMillis)
}

func sortedRequired(required map[capability]bool) []capability {
	values := make([]capability, 0, len(required))
	for value := range required {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values
}
