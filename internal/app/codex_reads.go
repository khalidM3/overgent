package app

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/stickguy/stickguy/internal/codexappserver"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/store"
)

// inferredReadPathsPerTurn bounds one turn-boundary refresh. It is larger than
// the in-turn hook bound because this runs after the agent's turn is over, but
// it is still bounded: a read set is not improved by an unbounded tail.
const inferredReadPathsPerTurn = 100

// codexReadRefreshBudget caps how long a turn boundary may spend recovering
// Codex reads. Measured against the bundled 0.149 app-server, a spawn,
// handshake, and thread read cost about 73ms; this leaves an order of magnitude
// of headroom and then gives up. Observation must never delay the coding agent
// (ADR-017), so a slow, missing, or broken Codex simply yields no read evidence.
const codexReadRefreshBudget = 2 * time.Second

// codexInferredReadsAvailable reports whether this device can reach a Codex
// able to answer for its own reads. Coverage must describe what is actually
// obtainable here, not what the vendor supports in principle: telling a member
// their Codex reads are covered when no Codex is installed would be the same
// silent overstatement this whole mechanism exists to remove.
func (s *Service) codexInferredReadsAvailable() bool {
	_, err := codexappserver.Locate()
	return err == nil
}

// codexInferredReadsUsable narrows codexInferredReadsAvailable from "a Codex is
// installed" to "a Codex is installed and the last refresh for this session
// actually worked".
//
// Locating the executable only proves a file is on disk. A Codex too old to
// answer, one that exits on start, a spawn that fails, or a thread read that
// exhausts codexReadRefreshBudget all leave the session with no recovered reads
// while Locate keeps succeeding — so coverage went on claiming vendor_inferred
// for a mechanism that was demonstrably not working. A member was then told
// their contract drift was covered while nothing was being observed, which is
// the exact silent overstatement ADR-052 exists to remove.
//
// A session whose refresh has never been attempted keeps the optimistic answer.
// That preserves the rule above these constants — a session with no reads yet
// still has the coverage its vendor allows — and means the downgrade is only
// ever made from evidence, never from a guess. The consequence is a one-event
// lag: the turn boundary whose refresh first fails has already reported its
// coverage, and every event after it reports none.
func (s *Service) codexInferredReadsUsable(sessionWorkstreamID string) bool {
	if !s.codexInferredReadsAvailable() {
		return false
	}
	s.codexReadHealthMu.Lock()
	defer s.codexReadHealthMu.Unlock()
	return !s.codexReadRefreshFailed[sessionWorkstreamID]
}

// recordCodexReadRefresh remembers whether the refresh mechanism answered for
// this session. Recovering zero reads is a success: a session that genuinely
// read nothing is not a broken mechanism, and treating it as one would silence
// coverage for a session that is working correctly.
func (s *Service) recordCodexReadRefresh(sessionWorkstreamID string, failed bool) {
	if sessionWorkstreamID == "" {
		return
	}
	s.codexReadHealthMu.Lock()
	defer s.codexReadHealthMu.Unlock()
	if s.codexReadRefreshFailed == nil {
		s.codexReadRefreshFailed = map[string]bool{}
	}
	s.codexReadRefreshFailed[sessionWorkstreamID] = failed
}

// publishCodexInferredReads recovers the file reads Codex's own classifier
// attributed to the commands this session ran, and adds them to the session's
// read set as vendor-inferred evidence (ADR-052).
//
// Codex exposes no file-reading tool, so no hook event ever names a file it
// read and a Codex session would otherwise have an empty read set and never
// receive a stale_assumption finding. The app-server's stored-task read is the
// supported way to recover that: the hook's session id is the app-server thread
// id, and reading a thread neither resumes it nor takes ownership of it.
//
// This is deliberately not complete. Codex classifies a command's actions
// best-effort, and a compound command that genuinely reads files can come back
// `unknown`, so what lands here is evidence of lower fidelity and is recorded
// as such rather than being presented as observation.
func (s *Service) publishCodexInferredReads(ctx context.Context, workspace config.Workspace, vendorSessionID, sessionWorkstreamID string) {
	if vendorSessionID == "" || sessionWorkstreamID == "" {
		return
	}
	budget, cancel := context.WithTimeout(ctx, codexReadRefreshBudget)
	defer cancel()
	client, err := codexappserver.Dial(budget, codexappserver.Options{})
	if err != nil {
		// A machine without Codex, or a Codex too old to answer, is an ordinary
		// condition. The session keeps its honest "no coverage" state, and the
		// failure is recorded so coverage stops claiming this device can infer
		// reads it cannot actually recover.
		s.recordCodexReadRefresh(sessionWorkstreamID, true)
		return
	}
	defer client.Close()
	cwd, reads, err := client.ThreadReads(budget, vendorSessionID)
	// A read that errors or exhausts the budget is a broken mechanism; one that
	// answers with no reads is a working mechanism reporting an empty result.
	s.recordCodexReadRefresh(sessionWorkstreamID, err != nil)
	if err != nil || len(reads) == 0 {
		return
	}
	// The thread must belong to this workspace. Codex hooks fire in every
	// repository the member opens, and a task read from another checkout must
	// never contribute paths here. This is a directory containment test, not a
	// path-safety one: the ordinary case is a thread whose working directory is
	// the workspace root itself.
	if cwd != "" && !withinRoot(workspace.Root, cwd) {
		return
	}
	// Codex reports absolute paths. publishReadSet re-derives and re-checks
	// every one against the workspace root, so a read of a file outside the
	// registered repository is dropped rather than recorded.
	candidates := make([]string, 0, len(reads))
	seen := make(map[string]bool, len(reads))
	for _, read := range reads {
		if seen[read.Path] {
			continue
		}
		seen[read.Path] = true
		candidates = append(candidates, read.Path)
	}
	s.publishReadSet(ctx, workspace, sessionWorkstreamID, candidates, store.ReadFidelityVendorInferred, inferredReadPathsPerTurn)
}

// withinRoot reports whether a directory is the workspace root or beneath it,
// after resolving symlinks on both sides so a linked checkout is not mistaken
// for an unrelated tree.
func withinRoot(root, directory string) bool {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		resolvedRoot = root
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		resolved = directory
	}
	relative, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return false
	}
	return relative == "." || !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".."
}
