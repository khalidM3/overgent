package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/khalidM3/overgent/internal/cliui"
)

func finding(id, state, delivery, severity string, workstreams ...string) map[string]any {
	ids := make([]any, 0, len(workstreams))
	for _, workstream := range workstreams {
		ids = append(ids, workstream)
	}
	return map[string]any{
		"id": id, "state": state, "delivery": delivery, "severity": severity,
		"reason": "Two sessions are editing auth.go", "workstreamIds": ids,
	}
}

func mine(ids ...string) map[string]bool {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// The single most dangerous output this product can produce is a false
// all-clear, so "clear" is only legal when every source actually answered.
func TestAttentionNeverClaimsClearFromMissingEvidence(t *testing.T) {
	both := evaluateAttention(nil, false, nil, false, mine("wrk_me"))
	if both.State != attentionUnavailable {
		t.Errorf("no evidence at all = %q, want %q", both.State, attentionUnavailable)
	}
	noFindings := evaluateAttention(nil, false, nil, true, mine("wrk_me"))
	if noFindings.State != attentionPartial {
		t.Errorf("findings missing = %q, want %q", noFindings.State, attentionPartial)
	}
	noSessions := evaluateAttention(nil, true, nil, false, mine("wrk_me"))
	if noSessions.State != attentionPartial {
		t.Errorf("sessions missing = %q, want %q", noSessions.State, attentionPartial)
	}
	clear := evaluateAttention(nil, true, nil, true, mine("wrk_me"))
	if clear.State != attentionClear {
		t.Errorf("both sources answered and nothing found = %q, want %q", clear.State, attentionClear)
	}
}

// A finding with no routing verdict predates the judgment layer or could not be
// judged. Treating it as `silent` would manufacture the all-clear above.
func TestAttentionReportsUnroutedFindingsAsCoverageNotSilence(t *testing.T) {
	result := evaluateAttention(
		[]map[string]any{finding("fnd_1", "open", "", "high", "wrk_me")},
		true, nil, true, mine("wrk_me"),
	)
	if result.State != attentionPartial {
		t.Fatalf("state = %q, want %q", result.State, attentionPartial)
	}
	if len(result.Items) != 0 {
		t.Errorf("an unrouted finding was presented as a routed one: %+v", result.Items)
	}
	if len(result.Gaps) != 1 || !strings.Contains(result.Gaps[0], "routing verdict") {
		t.Errorf("gaps = %v, want the unrouted finding named", result.Gaps)
	}
}

func TestAttentionRoutesOnlyNextTurnFindingsThatConvergeOnTheViewer(t *testing.T) {
	findings := []map[string]any{
		finding("fnd_mine", "open", "next_turn", "high", "wrk_me"),
		finding("fnd_dashboard", "open", "dashboard", "critical", "wrk_me"),
		finding("fnd_theirs", "open", "next_turn", "critical", "wrk_other"),
		finding("fnd_done", "resolved", "next_turn", "critical", "wrk_me"),
	}
	result := evaluateAttention(findings, true, nil, true, mine("wrk_me"))
	if result.State != attentionNeeded {
		t.Fatalf("state = %q", result.State)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "fnd_mine" {
		t.Fatalf("items = %+v, want only fnd_mine", result.Items)
	}
	// Only the finding on someone else's workstream is Elsewhere. A finding on
	// mine that judgment routed to the dashboard is simply not urgent, and a
	// resolved one is not work at all.
	if result.Elsewhere != 1 {
		t.Errorf("elsewhere = %d, want 1", result.Elsewhere)
	}
}

// A member owning two checkouts of one Project must still see work that lands
// on the other one: Needs you is scoped to the member, not to a directory.
func TestAttentionCountsEveryWorkstreamThisMemberOwns(t *testing.T) {
	result := evaluateAttention(
		[]map[string]any{finding("fnd_1", "open", "next_turn", "high", "wrk_second")},
		true, nil, true, mine("wrk_first", "wrk_second"),
	)
	if len(result.Items) != 1 {
		t.Fatalf("items = %+v", result.Items)
	}
}

func TestAttentionRanksFindingsBySeverityThenSessions(t *testing.T) {
	findings := []map[string]any{
		finding("fnd_low", "open", "next_turn", "low", "wrk_me"),
		finding("fnd_crit", "open", "next_turn", "critical", "wrk_me"),
	}
	sessions := []watchedSession{{Vendor: "codex", WorkstreamID: "wrk_me", Status: "waiting"}, {Vendor: "claude", Status: "active"}}
	result := evaluateAttention(findings, true, sessions, true, mine("wrk_me"))
	if len(result.Items) != 3 {
		t.Fatalf("items = %+v, want two findings and one waiting session", result.Items)
	}
	if result.Items[0].ID != "fnd_crit" || result.Items[1].ID != "fnd_low" {
		t.Errorf("findings not ranked by severity: %+v", result.Items)
	}
	// Section 6 puts routed findings ahead of vendor-reported session states.
	if result.Items[2].Source != "session" || result.Items[2].Status != "waiting" {
		t.Errorf("sessions did not sort after findings: %+v", result.Items)
	}
}

// A finding naming no workstream cannot be shown to converge on anyone, so it
// must not become work for whoever happens to be looking.
func TestAttentionDoesNotClaimUnattributedFindings(t *testing.T) {
	result := evaluateAttention([]map[string]any{
		{"id": "fnd_1", "state": "open", "delivery": "next_turn", "reason": "x"},
	}, true, nil, true, mine("wrk_me"))
	if len(result.Items) != 0 {
		t.Fatalf("items = %+v, want none", result.Items)
	}
	if result.Elsewhere != 1 {
		t.Errorf("elsewhere = %d, want 1", result.Elsewhere)
	}
}

func TestWriteAttentionDistinguishesClearFromUncertain(t *testing.T) {
	render := func(result attention) string {
		var output bytes.Buffer
		terminal := cliui.NewTerminal(cliui.Options{Out: &output, Color: cliui.ColorNever, Unicode: cliui.UnicodeNever, DefaultWidth: 80})
		if err := writeAttention(&output, terminal, result); err != nil {
			t.Fatal(err)
		}
		return output.String()
	}
	clear := render(attention{State: attentionClear})
	if !strings.Contains(clear, "Nothing needs you") {
		t.Errorf("clear state did not say so: %s", clear)
	}
	uncertain := render(attention{State: attentionPartial, Gaps: []string{"Project findings unavailable"}})
	if strings.Contains(uncertain, "Nothing needs you") {
		t.Errorf("an uncertain check reported an all-clear: %s", uncertain)
	}
	if !strings.Contains(uncertain, "could not check everything") || !strings.Contains(uncertain, "Project findings unavailable") {
		t.Errorf("uncertain state did not name the gap: %s", uncertain)
	}
	work := render(attention{State: attentionNeeded, Items: []attentionItem{{Source: "finding", Reason: "Two sessions are editing auth.go", Severity: "high"}}})
	for _, want := range []string{"NEEDS YOU", "Two sessions are editing auth.go", "high severity"} {
		if !strings.Contains(work, want) {
			t.Errorf("rendered attention missing %q:\n%s", want, work)
		}
	}
}

// The glance form counts work that does not converge on the viewer; it never
// competes with Needs you for the eye.
func TestWriteElsewhereCountStaysAGlance(t *testing.T) {
	render := func(count int) string {
		var output bytes.Buffer
		terminal := cliui.NewTerminal(cliui.Options{Out: &output, Color: cliui.ColorNever, Unicode: cliui.UnicodeNever, DefaultWidth: 80})
		if err := writeElsewhereCount(&output, terminal, count); err != nil {
			t.Fatal(err)
		}
		return output.String()
	}
	if render(0) != "" {
		t.Errorf("no other work should render nothing, got %q", render(0))
	}
	if !strings.Contains(render(1), "1 other finding in this Project") {
		t.Errorf("singular = %q", render(1))
	}
	if !strings.Contains(render(4), "4 other findings in this Project") {
		t.Errorf("plural = %q", render(4))
	}
}
