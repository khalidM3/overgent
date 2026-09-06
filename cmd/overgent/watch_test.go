package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/khalidM3/overgent/internal/cliui"
)

func TestRenderWatchSnapshotShowsAgentsFindingsAndCoverage(t *testing.T) {
	var output bytes.Buffer
	terminal := cliui.NewTerminal(cliui.Options{Out: &output, Color: cliui.ColorNever, Unicode: cliui.UnicodeNever, DefaultWidth: 80})
	snapshot := watchSnapshot{ProjectLabel: "Atlas", ObservedAt: time.Date(2026, 9, 5, 12, 0, 0, 0, time.Local), Sessions: []watchedSession{{Vendor: "codex", WorkstreamID: "wrk_1234567890123456", Status: "active"}}, Changes: []map[string]any{{"id": "fnd_1", "kind": "direct_collision", "severity": "high", "state": "open", "reason": "Two sessions are editing auth.go"}}, Coverage: []string{"team sessions unavailable"}}
	if err := renderWatchSnapshot(terminal, snapshot, nil, true); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Atlas", "AGENTS", "codex", "active", "ELSEWHERE", "Two sessions are editing auth.go", "direct collision"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("watch output missing %q:\n%s", want, output.String())
		}
	}
}

func TestWatchPriorities(t *testing.T) {
	if sessionPriority("waiting") <= sessionPriority("idle") {
		t.Fatal("waiting session must rank first")
	}
	if changePriority(map[string]any{"severity": "critical"}) <= changePriority(map[string]any{"severity": "low"}) {
		t.Fatal("critical finding must rank first")
	}
}

// A refresh that changes nothing must not redraw: the live view appends to
// scrollback, so an unchanged frame would be pure heartbeat noise.
func TestWatchFingerprintIgnoresObservationTimeAndFindingOrder(t *testing.T) {
	first := watchSnapshot{
		ObservedAt: time.Now(),
		Sessions:   []watchedSession{{Vendor: "codex", WorkstreamID: "wrk_1", Status: "active", ObservedAt: time.Now()}},
		Changes:    []map[string]any{{"id": "fnd_1", "state": "open", "severity": "high"}},
	}
	second := watchSnapshot{
		ObservedAt: time.Now().Add(90 * time.Second),
		Sessions:   []watchedSession{{Vendor: "codex", WorkstreamID: "wrk_1", Status: "active", ObservedAt: time.Now().Add(90 * time.Second)}},
		Changes:    []map[string]any{{"id": "fnd_1", "state": "open", "severity": "high"}},
	}
	if watchFingerprint(first) != watchFingerprint(second) {
		t.Fatal("a quiet refresh would have redrawn the live view")
	}
	moved := second
	moved.Changes = []map[string]any{{"id": "fnd_1", "state": "open", "severity": "critical"}}
	if watchFingerprint(second) == watchFingerprint(moved) {
		t.Fatal("a severity change must redraw")
	}
	gapped := second
	gapped.Coverage = []string{"Project findings unavailable"}
	if watchFingerprint(second) == watchFingerprint(gapped) {
		t.Fatal("losing coverage must redraw")
	}
}

// A one-shot snapshot must not claim to be live or offer Ctrl-C.
func TestRenderWatchSnapshotOnlyClaimsLiveWhileStreaming(t *testing.T) {
	snapshot := watchSnapshot{ProjectLabel: "Atlas", ObservedAt: time.Now()}
	var live, once bytes.Buffer
	terminal := func(w *bytes.Buffer) cliui.Terminal {
		return cliui.NewTerminal(cliui.Options{Out: w, Color: cliui.ColorNever, Unicode: cliui.UnicodeNever, DefaultWidth: 80})
	}
	if err := renderWatchSnapshot(terminal(&live), snapshot, nil, true); err != nil {
		t.Fatal(err)
	}
	if err := renderWatchSnapshot(terminal(&once), snapshot, nil, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(live.String(), "Ctrl-C") {
		t.Error("live view omits the stop affordance")
	}
	if strings.Contains(once.String(), "Ctrl-C") || strings.Contains(once.String(), "live") {
		t.Errorf("one-shot snapshot claimed to be live:\n%s", once.String())
	}
}

// The live view must never clear the screen; scrollback is the record.
func TestRenderWatchSnapshotNeverClearsTheScreen(t *testing.T) {
	var output bytes.Buffer
	terminal := cliui.NewTerminal(cliui.Options{Out: &output, Color: cliui.ColorNever, Unicode: cliui.UnicodeNever, DefaultWidth: 80})
	if err := renderWatchSnapshot(terminal, watchSnapshot{ProjectLabel: "Atlas", ObservedAt: time.Now()}, nil, true); err != nil {
		t.Fatal(err)
	}
	for _, escape := range []string{"\x1b[2J", "\x1b[H", "\x1b[?1049h"} {
		if strings.Contains(output.String(), escape) {
			t.Errorf("live view emitted screen-clearing escape %q", escape)
		}
	}
}

func footerTerminal(out *bytes.Buffer, isTTY bool) cliui.Terminal {
	return cliui.NewTerminal(cliui.Options{
		Out:        out,
		Color:      cliui.ColorNever,
		Unicode:    cliui.UnicodeNever,
		IsTerminal: func(any) bool { return isTTY },
		LookupEnv:  func(string) (string, bool) { return "", false },
	})
}

// The footer is a clock, not an indicator light (design-system rule 3), and it
// must rewrite one line rather than scroll: the frames above it are the record.
func TestWatchFooterRewritesOneLineAndNeverScrolls(t *testing.T) {
	var out bytes.Buffer
	start := time.Now()
	footer := &watchFooter{terminal: footerTerminal(&out, true), lastChange: start}
	footer.draw(start.Add(75*time.Second), false)
	text := out.String()
	if !strings.Contains(text, "quiet 1m 15s") {
		t.Errorf("footer did not show a counting clock: %q", text)
	}
	if strings.Contains(text, "\n") {
		t.Errorf("footer emitted a newline and would scroll: %q", text)
	}
	if !strings.Contains(text, "\r\x1b[2K") {
		t.Errorf("footer did not erase its own line: %q", text)
	}
	// Erasing one line is not clearing the screen.
	if strings.Contains(text, "\x1b[2J") {
		t.Errorf("footer cleared the screen: %q", text)
	}
}

// A spinner reassures while the service is unreachable. This line must not.
func TestWatchFooterSaysSoWhenCoverageIsIncomplete(t *testing.T) {
	var out bytes.Buffer
	start := time.Now()
	footer := &watchFooter{terminal: footerTerminal(&out, true), lastChange: start}
	footer.draw(start.Add(time.Second), true)
	if !strings.Contains(out.String(), "coverage incomplete") {
		t.Errorf("a degraded watch looked healthy: %q", out.String())
	}
}

// Cursor control must never reach a pipe, a file, or a dumb terminal.
func TestWatchFooterIsSilentWithoutARealTerminal(t *testing.T) {
	var out bytes.Buffer
	footer := &watchFooter{terminal: footerTerminal(&out, false), lastChange: time.Now()}
	footer.draw(time.Now(), false)
	footer.clear()
	footer.finish()
	if out.Len() != 0 {
		t.Errorf("footer wrote %q to a non-terminal", out.String())
	}
}

// Ctrl-C must leave the cursor on a clean line, not append the shell prompt to
// the footer text.
func TestWatchFooterFinishesOnACleanLine(t *testing.T) {
	var out bytes.Buffer
	footer := &watchFooter{terminal: footerTerminal(&out, true), lastChange: time.Now()}
	footer.draw(time.Now(), false)
	out.Reset()
	footer.finish()
	if out.String() != "\r\x1b[2K" {
		t.Errorf("finish left the line dirty: %q", out.String())
	}
	out.Reset()
	footer.finish()
	if out.Len() != 0 {
		t.Errorf("finish was not idempotent: %q", out.String())
	}
}
