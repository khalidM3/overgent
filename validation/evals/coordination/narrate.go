package main

import (
	"fmt"
	"sort"
	"strings"
)

/*
Narrated output for showing the coordination loop to a person.

The table is built for a gate: seven rows, pass or fail, one aggregate. It is
the wrong shape for a demonstration, where the interesting thing is not that
scenario A passed but what scenario A actually did - one agent moved a contract,
another had already read it, and the correction landed inside the second agent's
next turn without anyone relaying it. Everything printed here comes from the same
scenario report the gate writes; nothing is re-run and nothing is embellished.
*/

func narrateScenario(scenario scenarioReport) {
	fmt.Printf("\n── scenario %s · %s\n\n", scenario.ID, scenario.Name)

	if scenario.ExecutionError != "" {
		fmt.Printf("   could not run: %s\n", scenario.ExecutionError)
		return
	}

	if len(scenario.ActualFindings) == 0 {
		// Silence is a result, and in scenario E it is the required one, so it
		// is stated rather than left as an empty section the reader must read
		// as either success or breakage.
		fmt.Printf("   Stickguy said nothing.\n")
		fmt.Printf("   expected routing %s · actual %s\n", scenario.ExpectedRouting, scenario.ActualRouting)
		return
	}

	for _, finding := range scenario.ActualFindings {
		fmt.Printf("   %s  (%s severity, %s confidence)\n", finding.Kind, finding.Severity, finding.ConfidenceBand)
		for _, line := range wrapReason(finding.Reason, 68) {
			fmt.Printf("      %s\n", line)
		}
		if len(finding.Evidence) > 0 {
			fmt.Printf("      evidence: %s\n", strings.Join(evidenceLabels(finding.Evidence), ", "))
		}
		fmt.Println()
	}

	fmt.Printf("   routed %s", scenario.ActualRouting)
	if scenario.Metrics.DeliveryMillis > 0 {
		fmt.Printf(" · reached the affected session in %dms", scenario.Metrics.DeliveryMillis)
	}
	fmt.Println()
	fmt.Printf("   false interrupts %d · correct target %.0f%%\n", scenario.Metrics.FalseInterruptCount, scenario.Metrics.CorrectTargetRate*100)

	for _, assertion := range scenario.Assertions {
		mark := "·"
		switch assertion.Status {
		case statusPass:
			mark = "✓"
		case statusFail:
			mark = "✗"
		}
		fmt.Printf("   %s %s\n", mark, assertion.Name)
	}
}

// evidenceLabels pulls the human-readable label out of each evidence entry,
// falling back to the kind when a label is absent so an entry is never dropped
// silently from a demonstration.
func evidenceLabels(evidence []map[string]any) []string {
	var labels []string
	for _, entry := range evidence {
		if label, ok := entry["label"].(string); ok && label != "" {
			labels = append(labels, label)
			continue
		}
		if kind, ok := entry["kind"].(string); ok && kind != "" {
			labels = append(labels, kind)
		}
	}
	return labels
}

func wrapReason(reason string, width int) []string {
	words := strings.Fields(reason)
	if len(words) == 0 {
		return nil
	}
	lines := []string{words[0]}
	for _, word := range words[1:] {
		last := len(lines) - 1
		if len(lines[last])+1+len(word) <= width {
			lines[last] += " " + word
			continue
		}
		lines = append(lines, word)
	}
	return lines
}

// selectScenarios filters the definition list to the requested IDs, preserving
// the declared order so a multi-scenario demonstration always tells its story
// in the same sequence.
func selectScenarios(requested string) ([]scenarioDefinition, error) {
	if strings.TrimSpace(requested) == "" {
		return scenarioDefinitions, nil
	}
	wanted := map[string]bool{}
	for _, value := range strings.Split(requested, ",") {
		trimmed := strings.ToUpper(strings.TrimSpace(value))
		if trimmed != "" {
			wanted[trimmed] = true
		}
	}
	var selected []scenarioDefinition
	for _, definition := range scenarioDefinitions {
		if wanted[strings.ToUpper(definition.ID)] {
			selected = append(selected, definition)
			delete(wanted, strings.ToUpper(definition.ID))
		}
	}
	if len(wanted) > 0 {
		var unknown []string
		for id := range wanted {
			unknown = append(unknown, id)
		}
		sort.Strings(unknown)
		return nil, fmt.Errorf("unknown scenario %s", strings.Join(unknown, ", "))
	}
	return selected, nil
}
