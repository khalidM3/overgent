package app

import (
	"context"
	"time"

	"github.com/stickguy/stickguy/internal/agentactivity"
	"github.com/stickguy/stickguy/internal/codexappserver"
	"github.com/stickguy/stickguy/internal/daemon"
)

const (
	agentSessionLiveWindow    = 30 * time.Minute
	codexSessionResolveBudget = 750 * time.Millisecond
	codexThreadAmbiguityLimit = 2
)

type agentSessionResolution struct {
	WorkstreamID string `json:"workstreamId,omitempty"`
	Identified   bool   `json:"identified"`
	Ambiguous    bool   `json:"ambiguous"`
}

// handleAgentSessionResolution answers which supported agent session owns an
// MCP process in one working directory. A unique recent hook-derived session
// is authoritative local evidence. With no such evidence, Codex's read-only
// thread/list is the fallback. Two candidates at either layer are deliberately
// unidentified: cwd cannot distinguish two sessions in one checkout.
func (s *Service) handleAgentSessionResolution(ctx context.Context, q daemon.Request) daemon.Response {
	if q.AgentVendor != "codex" {
		return daemon.Response{OK: true, Data: agentSessionResolution{}}
	}
	cwd, ok := canonicalDirectory(q.AgentCWD)
	if !ok {
		return daemon.Response{OK: true, Data: agentSessionResolution{}}
	}
	workspace, ok := workspaceForCWD(s.cfg, cwd)
	if !ok {
		return daemon.Response{OK: true, Data: agentSessionResolution{}}
	}
	now := time.Now()
	sessions, err := s.store.ActiveAgentSessions(ctx, workspace.ID, "codex", cwd, now.Add(-agentSessionLiveWindow))
	if err != nil {
		return daemon.Response{OK: true, Data: agentSessionResolution{}}
	}
	if len(sessions) > 1 {
		return daemon.Response{OK: true, Data: agentSessionResolution{Ambiguous: true}}
	}
	if len(sessions) == 1 {
		return daemon.Response{OK: true, Data: agentSessionResolution{WorkstreamID: sessions[0].WorkstreamID, Identified: true}}
	}

	resolveContext, cancel := context.WithTimeout(ctx, codexSessionResolveBudget)
	defer cancel()
	threads, err := s.listCodexThreads(resolveContext, cwd, codexThreadAmbiguityLimit)
	if err != nil {
		return daemon.Response{OK: true, Data: agentSessionResolution{}}
	}
	cutoff := now.Add(-agentSessionLiveWindow).Unix()
	workstreams := map[string]bool{}
	for _, thread := range threads {
		threadCWD, valid := canonicalDirectory(thread.CWD)
		if !valid || threadCWD != cwd || thread.UpdatedAt < cutoff {
			continue
		}
		workstreamID, _, valid := agentactivity.WorkstreamIDFor("codex", thread.ID)
		if !valid {
			return daemon.Response{OK: true, Data: agentSessionResolution{}}
		}
		workstreams[workstreamID] = true
	}
	if len(workstreams) != 1 {
		return daemon.Response{OK: true, Data: agentSessionResolution{Ambiguous: len(workstreams) > 1}}
	}
	for workstreamID := range workstreams {
		return daemon.Response{OK: true, Data: agentSessionResolution{WorkstreamID: workstreamID, Identified: true}}
	}
	return daemon.Response{OK: true, Data: agentSessionResolution{}}
}

func (s *Service) listCodexThreads(ctx context.Context, cwd string, limit int) ([]codexappserver.Thread, error) {
	if s.codexThreadLister != nil {
		return s.codexThreadLister(ctx, cwd, limit)
	}
	client, err := codexappserver.Dial(ctx, codexappserver.Options{})
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.ListThreads(ctx, cwd, limit)
}
