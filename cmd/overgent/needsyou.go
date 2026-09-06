package main

import (
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/khalidM3/overgent/internal/cliui"
)

// This file is the CLI's answer to the product's first question: is anything
// about to hit me? Both `status` and `watch` render it, so the rule lives here
// once. Two implementations of "Needs you" would eventually disagree, and the
// one a member checked would be the wrong one.
//
// The hierarchy is docs/cli-experience.md section 6: findings the judgment
// layer routed `next_turn` onto one of this member's workstreams, then this
// member's vendor-reported waiting or error sessions. Findings that do not
// converge on the viewer are counted separately as Elsewhere.

// attentionState is deliberately four-valued. The separation between clear and
// partial is the whole point: section 6 forbids deriving an all-clear from
// missing evidence, so "nothing needs you" may only be said when every source
// actually answered.
const (
	attentionClear       = "clear"
	attentionNeeded      = "attention"
	attentionPartial     = "partial"
	attentionUnavailable = "unavailable"
)

type attentionItem struct {
	Source   string `json:"source"`
	ID       string `json:"id,omitempty"`
	Severity string `json:"severity,omitempty"`
	Reason   string `json:"reason"`
	Vendor   string `json:"vendor,omitempty"`
	Status   string `json:"status,omitempty"`
}

type attention struct {
	State     string          `json:"state"`
	Items     []attentionItem `json:"items,omitempty"`
	Elsewhere int             `json:"elsewhere"`
	Gaps      []string        `json:"gaps,omitempty"`
}

// evaluateAttention is pure so the hierarchy can be tested without a service,
// a backend, or a repository. findingsOK and sessionsOK report whether each
// source answered at all; a source that did not answer becomes a named gap
// rather than an absence of work.
func evaluateAttention(findings []map[string]any, findingsOK bool, sessions []watchedSession, sessionsOK bool, mine map[string]bool) attention {
	result := attention{State: attentionClear}
	if !findingsOK {
		result.Gaps = append(result.Gaps, "Project findings unavailable")
	}
	if !sessionsOK {
		result.Gaps = append(result.Gaps, "local agent sessions unavailable")
	}

	unrouted := 0
	if findingsOK {
		for _, finding := range findings {
			switch stringField(finding, "state") {
			case "resolved", "dismissed":
				continue
			}
			if !convergesOnViewer(finding, mine) {
				result.Elsewhere++
				continue
			}
			switch stringField(finding, "delivery") {
			case "next_turn":
				result.Items = append(result.Items, attentionItem{
					Source:   "finding",
					ID:       stringField(finding, "id"),
					Severity: stringField(finding, "severity"),
					Reason:   stringField(finding, "reason", "text", "summary"),
				})
			case "":
				// A finding on this member's workstream that carries no routing
				// verdict is genuinely undecided: it predates the judgment
				// layer, or judgment could not run. Reading that as `silent`
				// would manufacture the all-clear section 6 prohibits, so it is
				// reported as missing coverage instead of quietly dropped.
				unrouted++
			}
		}
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		return severityRank(result.Items[i].Severity) > severityRank(result.Items[j].Severity)
	})

	if sessionsOK {
		for _, session := range sessions {
			if session.Status != "waiting" && session.Status != "error" {
				continue
			}
			result.Items = append(result.Items, attentionItem{
				Source: "session",
				Reason: sessionAttentionReason(session.Status),
				Vendor: session.Vendor,
				Status: session.Status,
				ID:     session.WorkstreamID,
			})
		}
	}

	if unrouted > 0 {
		result.Gaps = append(result.Gaps, pluralizeUnrouted(unrouted))
	}
	switch {
	case len(result.Items) > 0:
		result.State = attentionNeeded
	case !findingsOK && !sessionsOK:
		result.State = attentionUnavailable
	case len(result.Gaps) > 0:
		result.State = attentionPartial
	}
	return result
}

// convergesOnViewer reports whether a finding names one of this member's
// workstreams on this device. A finding with no workstreams named cannot be
// shown to converge, so it counts as Elsewhere rather than as work for whoever
// happens to be looking.
func convergesOnViewer(finding map[string]any, mine map[string]bool) bool {
	raw, ok := finding["workstreamIds"].([]any)
	if !ok {
		return false
	}
	for _, value := range raw {
		if id, isString := value.(string); isString && mine[id] {
			return true
		}
	}
	return false
}

func sessionAttentionReason(status string) string {
	if status == "error" {
		return "An agent session reported an error"
	}
	return "An agent session is waiting on you"
}

func pluralizeUnrouted(count int) string {
	if count == 1 {
		return "1 finding on your work has no routing verdict"
	}
	return strconv.Itoa(count) + " findings on your work have no routing verdict"
}

func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}

// writeAttention renders the hierarchy for a person. Alert styling marks only
// work converging on the viewer, per the visual grammar in section 7; coverage
// gaps and Elsewhere stay neutral because neither is a call to act.
func writeAttention(stdout io.Writer, terminal cliui.Terminal, result attention) error {
	if _, err := fmt.Fprintln(stdout, terminal.Style(cliui.StyleBold, "NEEDS YOU")); err != nil {
		return err
	}
	switch result.State {
	case attentionNeeded:
		for _, item := range result.Items {
			mark := terminal.Style(cliui.StyleAlert, terminal.Symbol("●", "!"))
			if _, err := fmt.Fprintf(stdout, "  %s %s\n", mark, item.Reason); err != nil {
				return err
			}
			if detail := attentionDetail(item); detail != "" {
				if _, err := fmt.Fprintf(stdout, "    %s\n", terminal.Style(cliui.StyleMuted, detail)); err != nil {
					return err
				}
			}
		}
	case attentionClear:
		if _, err := fmt.Fprintf(stdout, "  %s %s\n", terminal.Symbol("○", "-"), terminal.Style(cliui.StyleMuted, "Nothing needs you right now")); err != nil {
			return err
		}
	default:
		// Neither "nothing needs you" nor a list: the CLI could not see enough
		// to say. Naming that plainly is the point.
		if _, err := fmt.Fprintf(stdout, "  %s %s\n", terminal.Symbol("○", "-"), terminal.Style(cliui.StyleMuted, "Overgent could not check everything")); err != nil {
			return err
		}
	}
	for _, gap := range result.Gaps {
		if _, err := fmt.Fprintf(stdout, "  %s %s\n", terminal.Symbol("○", "-"), terminal.Style(cliui.StyleMuted, gap)); err != nil {
			return err
		}
	}
	return nil
}

// writeElsewhereCount is the glance form of section 6's Elsewhere block: work
// that is real but does not converge on the viewer, so it is a count in neutral
// styling rather than a list competing with Needs you. A live stream has room
// to name the findings instead, so `watch` renders its own.
func writeElsewhereCount(stdout io.Writer, terminal cliui.Terminal, count int) error {
	if count <= 0 {
		return nil
	}
	label := "1 other finding in this Project"
	if count > 1 {
		label = strconv.Itoa(count) + " other findings in this Project"
	}
	_, err := fmt.Fprintf(stdout, "\n%s\n  %s\n", terminal.Style(cliui.StyleBold, "ELSEWHERE"), terminal.Style(cliui.StyleMuted, label))
	return err
}

func attentionDetail(item attentionItem) string {
	if item.Source == "session" {
		if item.Vendor == "" {
			return ""
		}
		return item.Vendor
	}
	if item.Severity == "" {
		return ""
	}
	return item.Severity + " severity"
}
