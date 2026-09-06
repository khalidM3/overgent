package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/khalidM3/overgent/internal/cliui"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/credential"
	"github.com/khalidM3/overgent/internal/daemon"
	"github.com/khalidM3/overgent/internal/hosted"
)

type watchedSession struct {
	Vendor, WorkstreamID, Status string
	ObservedAt                   time.Time
}

type watchSnapshot struct {
	SchemaVersion int              `json:"schemaVersion"`
	ObservedAt    time.Time        `json:"observedAt"`
	ProjectID     string           `json:"projectId"`
	ProjectLabel  string           `json:"projectLabel,omitempty"`
	Sessions      []watchedSession `json:"sessions"`
	Changes       []map[string]any `json:"changes"`
	// NeedsYou applies the same hierarchy `status` renders, so a member
	// watching a Project and a member glancing at it are answered by one rule.
	NeedsYou attention `json:"needsYou"`
	Coverage []string  `json:"coverage,omitempty"`
}

func runWatch(ctx context.Context, paths config.Paths, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("watch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	projectID := flags.String("project", "", "select a Project by id")
	jsonLines := flags.Bool("jsonl", false, "emit one JSON snapshot per refresh")
	// --json is accepted only so a member who reached for the read-command flag
	// gets the streaming spelling back, instead of a flag-package usage dump.
	jsonDocument := flags.Bool("json", false, "not valid while streaming; use --jsonl")
	once := flags.Bool("once", false, "render one snapshot and exit")
	interval := flags.Duration("interval", 2*time.Second, "refresh interval")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("watch accepts flags only")
	}
	if *jsonDocument && !*once {
		return errors.New("a live stream has no single JSON document.\n\nNext: use `--jsonl` for one record per refresh, or `--once --json` for a single snapshot")
	}
	if *jsonDocument {
		*jsonLines = true
	}
	if *interval < 500*time.Millisecond {
		return errors.New("watch interval must be at least 500ms")
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}
	workspace, err := selectStatusWorkspace(cfg, *projectID, os.Getwd)
	if err != nil {
		return err
	}
	backend, ok := cfg.BackendForWorkspace(workspace)
	if !ok {
		return errors.New("The selected Project has no configured backend.\n\nNext: run `overgent doctor`")
	}
	// The Project backend supplies findings; local agent sessions come from the
	// service. Losing one must degrade that section rather than the command, so
	// an unreachable backend is recorded as missing coverage, not an exit.
	var client *hosted.Client
	if token, credentialErr := credential.Get(ctx, backend.DeviceID); credentialErr == nil {
		client, _ = hosted.New(backend.APIBaseURL, token)
	}
	label := workspace.ProjectID
	if client != nil {
		if bootstrap, bootstrapErr := client.Bootstrap(ctx); bootstrapErr == nil {
			for _, project := range bootstrap.Projects {
				if project.ID == workspace.ProjectID && project.Label != "" {
					label = project.Label
				}
			}
		}
	}
	watchCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	terminal := presentationTerminal(stdin, stdout, stderr)
	known := map[string]map[string]any{}
	viewer := viewerWorkstreams(cfg, workspace.ProjectID)
	lastRendered := ""
	live := !*jsonLines && !*once
	footer := &watchFooter{terminal: terminal, lastChange: time.Now()}
	if live {
		defer footer.finish()
	}
	for {
		snapshot := watchSnapshot{SchemaVersion: cliOutputSchemaVersion, ObservedAt: time.Now().UTC(), ProjectID: workspace.ProjectID, ProjectLabel: label}
		response, callErr := daemon.Call(watchCtx, paths.Socket, daemon.Request{Method: "project_activity", WorkspaceID: workspace.ID})
		sessionsOK := callErr == nil && response.OK
		if sessionsOK {
			snapshot.Sessions = decodeWatchedSessions(response.Data)
		}
		findingsOK := false
		if client == nil {
			// no findings source
		} else if page, changeErr := client.ProjectChanges(watchCtx, workspace.ProjectID); changeErr == nil {
			findingsOK = true
			for _, item := range page.Items {
				id, _ := item["id"].(string)
				if id == "" {
					continue
				}
				// Settled work is dropped rather than remembered: the map is
				// the live set, so a long watch cannot grow without bound.
				if state, _ := item["state"].(string); state == "resolved" || state == "dismissed" {
					delete(known, id)
					continue
				}
				known[id] = item
			}
		}
		for _, item := range known {
			snapshot.Changes = append(snapshot.Changes, item)
		}
		// Map iteration is unordered, so equal-severity findings would swap
		// places on every refresh. Rank by severity, then by id, so the list
		// only moves when the Project actually moved.
		sort.Slice(snapshot.Changes, func(i, j int) bool {
			left, right := changePriority(snapshot.Changes[i]), changePriority(snapshot.Changes[j])
			if left != right {
				return left > right
			}
			return stringField(snapshot.Changes[i], "id") < stringField(snapshot.Changes[j], "id")
		})
		snapshot.NeedsYou = evaluateAttention(snapshot.Changes, findingsOK, snapshot.Sessions, sessionsOK, viewer)
		snapshot.Coverage = snapshot.NeedsYou.Gaps
		if *jsonLines {
			if err = json.NewEncoder(stdout).Encode(snapshot); err != nil {
				return err
			}
		} else if fingerprint := watchFingerprint(snapshot); *once || fingerprint != lastRendered {
			lastRendered = fingerprint
			// The footer is erased before the frame is appended, so the frame
			// lands on a clean line and the footer is redrawn beneath it.
			footer.clear()
			if err = renderWatchSnapshot(terminal, snapshot, viewer, !*once); err != nil {
				return err
			}
			footer.lastChange = time.Now()
		}
		if *once {
			return nil
		}
		degraded := len(snapshot.Coverage) > 0
		// The clock advances once per second regardless of the poll interval,
		// so a member reading the line sees time move even when the Project is
		// quiet and nothing is being fetched.
		next := time.After(*interval)
		for waiting := true; waiting; {
			if live {
				footer.draw(time.Now(), degraded)
			}
			select {
			case <-watchCtx.Done():
				return nil
			case <-next:
				waiting = false
			case <-time.After(time.Second):
			}
		}
	}
}

func decodeWatchedSessions(data any) []watchedSession {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var envelope struct {
		Sessions []struct {
			Vendor, WorkstreamID, Status string
			ObservedAt                   time.Time
		}
	}
	if json.Unmarshal(encoded, &envelope) != nil {
		return nil
	}
	out := make([]watchedSession, 0, len(envelope.Sessions))
	for _, session := range envelope.Sessions {
		out = append(out, watchedSession{session.Vendor, session.WorkstreamID, session.Status, session.ObservedAt})
	}
	sort.Slice(out, func(i, j int) bool { return sessionPriority(out[i].Status) > sessionPriority(out[j].Status) })
	return out
}

// watchFingerprint captures everything the live view actually shows. Refreshes
// that do not change it are dropped, which is what keeps a long watch quiet and
// its scrollback readable (cli-experience.md section 2).
func watchFingerprint(snapshot watchSnapshot) string {
	var b strings.Builder
	for _, session := range snapshot.Sessions {
		fmt.Fprintf(&b, "s|%s|%s|%s\n", session.Vendor, session.WorkstreamID, session.Status)
	}
	for _, item := range snapshot.Changes {
		fmt.Fprintf(&b, "c|%s|%s|%s\n", stringField(item, "id"), stringField(item, "state"), stringField(item, "severity"))
	}
	for _, gap := range snapshot.Coverage {
		fmt.Fprintf(&b, "g|%s\n", gap)
	}
	fmt.Fprintf(&b, "n|%s|%d\n", snapshot.NeedsYou.State, snapshot.NeedsYou.Elsewhere)
	return b.String()
}

// renderWatchSnapshot appends one frame. It never clears the screen: the live
// view is a chronological stream a member can scroll back through, not an
// alternate-screen TUI (cli-experience.md sections 2 and 7).
func renderWatchSnapshot(terminal cliui.Terminal, snapshot watchSnapshot, viewer map[string]bool, live bool) error {
	// No "live" mark in the frame. Liveness belongs to the footer clock, which
	// is the only thing on screen that is still true a second from now; a dot
	// printed here would be an indicator light (design-system rule 3), and in
	// scrollback it would claim a past frame is current.
	hint := "Watching this Project · Ctrl-C to stop"
	if !live {
		hint = "Snapshot of this Project"
	}
	fmt.Fprintf(terminal.Out(), "%s\n%s\n\n", terminal.Style(cliui.StyleBold, snapshot.ProjectLabel), terminal.Style(cliui.StyleMuted, hint))
	if err := writeAttention(terminal.Out(), terminal, snapshot.NeedsYou); err != nil {
		return err
	}
	fmt.Fprintln(terminal.Out())
	fmt.Fprintln(terminal.Out(), terminal.Style(cliui.StyleBold, "AGENTS"))
	if len(snapshot.Sessions) == 0 {
		fmt.Fprintln(terminal.Out(), "  "+terminal.Style(cliui.StyleMuted, "No recently observed local agent sessions"))
	}
	for _, session := range snapshot.Sessions {
		state := terminal.Style(cliui.StyleLive, session.Status)
		if session.Status == "waiting" || session.Status == "error" {
			state = terminal.Style(cliui.StyleAlert, session.Status)
		}
		if session.Status == "idle" {
			state = terminal.Style(cliui.StyleMuted, session.Status)
		}
		alias := session.WorkstreamID
		if len(alias) > 12 {
			alias = alias[len(alias)-12:]
		}
		fmt.Fprintf(terminal.Out(), "  %s  %-8s %s  %s\n", terminal.Symbol("●", "*"), session.Vendor, state, terminal.Style(cliui.StyleMuted, alias))
	}
	// Elsewhere lists rather than counts here: a live stream has the room, and
	// work that does not converge on the viewer is still worth watching. Alert
	// styling is deliberately absent — section 7 reserves it for work that
	// converges on the viewer, which is what NEEDS YOU above already carries.
	fmt.Fprintln(terminal.Out())
	fmt.Fprintln(terminal.Out(), terminal.Style(cliui.StyleBold, "ELSEWHERE"))
	visible := 0
	for _, item := range snapshot.Changes {
		state, _ := item["state"].(string)
		if state == "resolved" || state == "dismissed" {
			continue
		}
		if convergesOnViewer(item, viewer) {
			continue
		}
		reason := stringField(item, "reason", "text", "summary")
		if reason == "" {
			continue
		}
		kind := strings.ReplaceAll(stringField(item, "kind"), "_", " ")
		fmt.Fprintf(terminal.Out(), "  %s %s\n", terminal.Symbol("·", "-"), reason)
		if kind != "" {
			fmt.Fprintf(terminal.Out(), "    %s\n", terminal.Style(cliui.StyleMuted, kind))
		}
		visible++
	}
	if visible == 0 {
		fmt.Fprintln(terminal.Out(), "  "+terminal.Style(cliui.StyleMuted, "Nothing else open in this Project"))
	}
	return nil
}

// watchFooter is the live line at the bottom of a watch. Design-system rule 3
// forbids an indicator light: running work reads as a clock that counts up and
// text that moves, never a spinner. So the footer states two facts that are
// true right now — how long this Project has been quiet, and whether the last
// poll could see everything — and the clock advances once per second.
//
// A spinner would be worse than useless here. It spins just as happily when the
// service is unreachable, which is exactly the state this command exists to
// surface. The clock stops being reassuring the moment coverage degrades.
type watchFooter struct {
	terminal   cliui.Terminal
	shown      bool
	lastChange time.Time
}

func (footer *watchFooter) clear() {
	if footer.shown && footer.terminal.Animated() {
		fmt.Fprint(footer.terminal.Out(), "\r\x1b[2K")
	}
	footer.shown = false
}

// draw rewrites the footer in place. It writes no newline, so the line is
// overwritten on the next tick instead of scrolling: the frames above it stay
// exactly as they were printed and scrollback remains the record.
func (footer *watchFooter) draw(now time.Time, degraded bool) {
	if !footer.terminal.Animated() {
		return
	}
	mark, state := footer.terminal.Symbol("●", "*"), cliui.StyleLive
	if degraded {
		mark, state = footer.terminal.Symbol("○", "-"), cliui.StyleMuted
	}
	quiet := "quiet " + cliui.FormatElapsed(now.Sub(footer.lastChange))
	if degraded {
		quiet += " · coverage incomplete"
	}
	fmt.Fprintf(footer.terminal.Out(), "\r\x1b[2K%s %s",
		footer.terminal.Style(state, mark),
		footer.terminal.Style(cliui.StyleMuted, quiet))
	footer.shown = true
}

// finish leaves the cursor on a clean line so the shell prompt is not appended
// to the footer when a member presses Ctrl-C.
func (footer *watchFooter) finish() {
	if footer.shown && footer.terminal.Animated() {
		fmt.Fprint(footer.terminal.Out(), "\r\x1b[2K")
	}
	footer.shown = false
}

func stringField(item map[string]any, names ...string) string {
	for _, name := range names {
		if value, ok := item[name].(string); ok {
			return value
		}
	}
	return ""
}
func changePriority(item map[string]any) int {
	switch stringField(item, "severity") {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	}
	return 1
}
func sessionPriority(state string) int {
	switch state {
	case "error", "waiting":
		return 4
	case "active":
		return 3
	case "idle":
		return 2
	}
	return 1
}
