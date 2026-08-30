package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stickguy/stickguy/internal/agentactivity"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
	"github.com/stickguy/stickguy/internal/hosted"
)

const instructions = "Stickguy is advisory only. Before broad/shared edits, call begin_work then check_coordination; read relevant findings and resolutions. Use get_resolutions when a collision affecting this workstream has been resolved. Report bounded checkpoints; finish_work before completion. Never send source, diffs, env values, command lines, raw tool/test output, or secrets. Project membership and the pause switch govern sharing; the secret classifier is mandatory. Fail on workspace ambiguity. Stickguy never edits Git, runs coding tools, controls agents, or authorizes teammate mutations."

type commonInput struct {
	WorkspaceID string `json:"workspace_id,omitempty" jsonschema:"explicit registered workspace ID; omit when cwd resolves uniquely"`
}
type intentInput struct {
	commonInput
	IdempotencyKey   string   `json:"idempotency_key" jsonschema:"stable retry key, 1-128 characters"`
	Revision         int64    `json:"revision,omitempty" jsonschema:"current intent revision; required for update_intent"`
	Title            string   `json:"title" jsonschema:"bounded workstream title"`
	Outcome          string   `json:"outcome" jsonschema:"bounded intended behavioral outcome"`
	Approach         string   `json:"approach,omitempty" jsonschema:"bounded approach summary; never source or raw output"`
	Components       []string `json:"components,omitempty"`
	Contracts        []string `json:"contracts,omitempty"`
	WaitingOn        []string `json:"waiting_on,omitempty" jsonschema:"at most 8 bounded contract, symbol, or path claims"`
	AnticipatedPaths []string `json:"anticipated_paths,omitempty"`
	PlanItemIDs      []string `json:"plan_item_ids,omitempty"`
}
type checkInput struct {
	commonInput
	Trigger                string `json:"trigger,omitempty" jsonschema:"begin, before_broad_edit, checkpoint, refresh, finish, or manual"`
	SinceCursor            string `json:"since_cursor,omitempty"`
	ApproximateTokenBudget int64  `json:"approximate_token_budget,omitempty" jsonschema:"128-800; defaults to 400"`
}
type verificationInput struct {
	State             string `json:"state"`
	CheckKind         string `json:"check_kind"`
	Label             string `json:"label"`
	Summary           string `json:"summary"`
	AffectedComponent string `json:"affected_component,omitempty"`
	ObservedAt        string `json:"observed_at,omitempty"`
	ManifestRevision  int64  `json:"manifest_revision,omitempty"`
}
type checkpointInput struct {
	commonInput
	CheckpointID     string              `json:"checkpoint_id"`
	Summary          string              `json:"summary" jsonschema:"bounded behavioral summary; never raw output"`
	Discoveries      []string            `json:"discoveries,omitempty"`
	Verification     []verificationInput `json:"verification,omitempty"`
	ManifestRevision int64               `json:"manifest_revision,omitempty"`
	BasedOnBriefID   string              `json:"based_on_brief_id,omitempty"`
}
type acknowledgeInput struct {
	commonInput
	IdempotencyKey    string   `json:"idempotency_key,omitempty"`
	BriefID           string   `json:"brief_id"`
	ConsideredItemIDs []string `json:"considered_item_ids"`
}
type finishInput struct {
	commonInput
	IdempotencyKey   string              `json:"idempotency_key"`
	Outcome          string              `json:"outcome"`
	Summary          string              `json:"summary"`
	Verification     []verificationInput `json:"verification,omitempty"`
	ManifestRevision int64               `json:"manifest_revision,omitempty"`
	BasedOnBriefID   string              `json:"based_on_brief_id,omitempty"`
}
type eventInput struct {
	commonInput
	IdempotencyKey string `json:"idempotency_key"`
	Kind           string `json:"kind" jsonschema:"decision, completion, or blocker"`
	Summary        string `json:"summary" jsonschema:"bounded summary; never raw output"`
}
type collaborationReadInput struct {
	commonInput
	SinceRevision int64 `json:"since_revision,omitempty"`
}
type toolOutput struct {
	ProjectID      string                        `json:"projectId"`
	WorkspaceID    string                        `json:"workspaceId"`
	WorkstreamID   string                        `json:"workstreamId"`
	Duplicate      bool                          `json:"duplicate"`
	IntentRevision int64                         `json:"intentRevision,omitempty"`
	Brief          *hosted.CoordinationBrief     `json:"brief,omitempty"`
	Degraded       bool                          `json:"degraded"`
	Degradation    string                        `json:"degradation,omitempty"`
	Collaboration  *hosted.CollaborationSnapshot `json:"collaboration,omitempty"`
}

type server struct {
	paths config.Paths
	cfg   config.Config
	// agentWorkstreamID identifies the coding-agent session that spawned this
	// MCP server, when the vendor exposes one. Empty means the session could
	// not be identified and lifecycle calls fall back to the workspace
	// workstream, which is honest but cannot see session-routed findings.
	agentWorkstreamID string
	cwd               string
	daemonCall        daemonCaller
	sessionMu         sync.Mutex
}

type daemonCaller func(context.Context, string, daemon.Request) (daemon.Response, error)

var agentSessionWorkstreamID = regexp.MustCompile(`^wrk_agent_[0-9a-f]{32}$`)

// sessionWorkstream recovers the calling agent session's workstream identity
// from the environment the vendor gave this process. Claude Code exports
// CLAUDE_CODE_SESSION_ID. When it is absent, the local service resolves one
// live Codex thread by this MCP process's cwd. Empty means zero or ambiguous;
// it is never guessed. The vendor id stays local and only its derived
// workstream identity is returned.
func sessionWorkstream(ctx context.Context, socket, cwd string, call daemonCaller) string {
	if sessionID := os.Getenv("CLAUDE_CODE_SESSION_ID"); sessionID != "" {
		if workstreamID, _, ok := agentactivity.WorkstreamIDFor("claude", sessionID); ok {
			return workstreamID
		}
		return ""
	}
	if socket == "" || cwd == "" || call == nil {
		return ""
	}
	resolveContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	response, err := call(resolveContext, socket, daemon.Request{Method: "resolve_agent_session", AgentVendor: "codex", AgentCWD: cwd})
	if err != nil || !response.OK {
		return ""
	}
	var resolution struct {
		WorkstreamID string `json:"workstreamId"`
		Identified   bool   `json:"identified"`
	}
	encoded, err := json.Marshal(response.Data)
	if err != nil || json.Unmarshal(encoded, &resolution) != nil || !resolution.Identified || !agentSessionWorkstreamID.MatchString(resolution.WorkstreamID) {
		return ""
	}
	return resolution.WorkstreamID
}

func Run(ctx context.Context, configRoot string) error {
	paths, err := config.Resolve(configRoot)
	if err != nil {
		return err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	bridge := &server{paths: paths, cfg: cfg, cwd: cwd, daemonCall: daemon.Call}
	sdk := newSDK(bridge)
	return sdk.Run(ctx, &sdkmcp.StdioTransport{})
}

func newSDK(bridge *server) *sdkmcp.Server {
	sdk := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "stickguy", Title: "Stickguy coordination harness", Version: "0.1.0"}, &sdkmcp.ServerOptions{Instructions: instructions, Capabilities: &sdkmcp.ServerCapabilities{}})
	sdkmcp.AddTool(sdk, &sdkmcp.Tool{Name: "begin_work", Description: "Begin or resume a workstream, publish bounded intent idempotently, and return relevant coordination context."}, bridge.beginWork)
	sdkmcp.AddTool(sdk, &sdkmcp.Tool{Name: "update_intent", Description: "Revision-check and update active bounded intent without sending source, diffs, prompts, or raw output."}, bridge.updateIntent)
	sdkmcp.AddTool(sdk, &sdkmcp.Tool{Name: "check_coordination", Description: "Read a bounded relevant coordination brief before broad/shared edits or on refresh."}, bridge.checkCoordination)
	sdkmcp.AddTool(sdk, &sdkmcp.Tool{Name: "report_checkpoint", Description: "Publish an idempotent behavioral checkpoint with structured verification metadata and return newly relevant context."}, bridge.reportCheckpoint)
	sdkmcp.AddTool(sdk, &sdkmcp.Tool{Name: "acknowledge_context", Description: "Record which brief items were considered; this never claims compliance or correctness."}, bridge.acknowledgeContext)
	sdkmcp.AddTool(sdk, &sdkmcp.Tool{Name: "finish_work", Description: "Mark the workstream done idempotently and return unresolved relevant context before completion."}, bridge.finishWork)
	sdkmcp.AddTool(sdk, &sdkmcp.Tool{Name: "report_event", Description: "Report one bounded decision, completion, or blocker summary."}, bridge.reportEvent)
	sdkmcp.AddTool(sdk, &sdkmcp.Tool{Name: "get_resolutions", Description: "Read how collisions affecting this workstream were resolved."}, bridge.getResolutions)
	return sdk
}

func (s *server) beginWork(ctx context.Context, _ *sdkmcp.CallToolRequest, in intentInput) (*sdkmcp.CallToolResult, toolOutput, error) {
	return s.intent(ctx, "begin_work", in)
}
func (s *server) updateIntent(ctx context.Context, _ *sdkmcp.CallToolRequest, in intentInput) (*sdkmcp.CallToolResult, toolOutput, error) {
	return s.intent(ctx, "update_intent", in)
}
func (s *server) intent(ctx context.Context, method string, in intentInput) (*sdkmcp.CallToolResult, toolOutput, error) {
	if err := validateWaitingOn(in.WaitingOn); err != nil {
		return nil, toolOutput{}, err
	}
	q := daemon.Request{Method: method, WorkspaceID: in.WorkspaceID, IdempotencyKey: in.IdempotencyKey, Revision: in.Revision, Title: in.Title, IntendedOutcome: in.Outcome, ApproachSummary: in.Approach, Components: in.Components, Contracts: in.Contracts, WaitingOn: in.WaitingOn, AnticipatedPaths: in.AnticipatedPaths, PlanItemIDs: in.PlanItemIDs}
	out, err := s.call(ctx, &q)
	return nil, out, err
}

func validateWaitingOn(values []string) error {
	if len(values) > 8 {
		return errors.New("waiting_on exceeds 8 claims")
	}
	for _, value := range values {
		if len(value) < 1 || len(value) > 160 || strings.ContainsAny(value, "\r\n\x00") {
			return errors.New("waiting_on claim must be 1-160 safe characters")
		}
	}
	return nil
}
func (s *server) checkCoordination(ctx context.Context, _ *sdkmcp.CallToolRequest, in checkInput) (*sdkmcp.CallToolResult, toolOutput, error) {
	q := daemon.Request{Method: "check_coordination", WorkspaceID: in.WorkspaceID, Trigger: in.Trigger, SinceCursor: in.SinceCursor, ApproximateTokenBudget: in.ApproximateTokenBudget}
	out, err := s.call(ctx, &q)
	return nil, out, err
}
func (s *server) reportCheckpoint(ctx context.Context, _ *sdkmcp.CallToolRequest, in checkpointInput) (*sdkmcp.CallToolResult, toolOutput, error) {
	q := daemon.Request{Method: "report_checkpoint", WorkspaceID: in.WorkspaceID, CheckpointID: in.CheckpointID, Summary: in.Summary, Discoveries: in.Discoveries, ManifestRevision: in.ManifestRevision, BriefID: in.BasedOnBriefID, Verification: verification(in.Verification)}
	out, err := s.call(ctx, &q)
	return nil, out, err
}
func (s *server) acknowledgeContext(ctx context.Context, _ *sdkmcp.CallToolRequest, in acknowledgeInput) (*sdkmcp.CallToolResult, toolOutput, error) {
	q := daemon.Request{Method: "acknowledge_context", WorkspaceID: in.WorkspaceID, IdempotencyKey: in.IdempotencyKey, BriefID: in.BriefID, ConsideredItemIDs: in.ConsideredItemIDs}
	out, err := s.call(ctx, &q)
	return nil, out, err
}
func (s *server) finishWork(ctx context.Context, _ *sdkmcp.CallToolRequest, in finishInput) (*sdkmcp.CallToolResult, toolOutput, error) {
	q := daemon.Request{Method: "finish_work", WorkspaceID: in.WorkspaceID, IdempotencyKey: in.IdempotencyKey, Outcome: in.Outcome, Summary: in.Summary, ManifestRevision: in.ManifestRevision, BriefID: in.BasedOnBriefID, Verification: verification(in.Verification)}
	out, err := s.call(ctx, &q)
	return nil, out, err
}
func (s *server) reportEvent(ctx context.Context, _ *sdkmcp.CallToolRequest, in eventInput) (*sdkmcp.CallToolResult, toolOutput, error) {
	q := daemon.Request{Method: "report_event", WorkspaceID: in.WorkspaceID, IdempotencyKey: in.IdempotencyKey, Kind: in.Kind, Summary: in.Summary}
	out, err := s.call(ctx, &q)
	return nil, out, err
}
func (s *server) getResolutions(ctx context.Context, _ *sdkmcp.CallToolRequest, in collaborationReadInput) (*sdkmcp.CallToolResult, toolOutput, error) {
	return s.collaborationCall(ctx, daemon.Request{Method: "get_resolutions", WorkspaceID: in.WorkspaceID, SinceRevision: in.SinceRevision})
}
func (s *server) collaborationCall(ctx context.Context, q daemon.Request) (*sdkmcp.CallToolResult, toolOutput, error) {
	out, err := s.call(ctx, &q)
	return nil, out, err
}

func (s *server) call(ctx context.Context, q *daemon.Request) (toolOutput, error) {
	workspace, err := s.resolveWorkspace(q.WorkspaceID)
	if err != nil {
		return toolOutput{}, err
	}
	q.WorkspaceID = workspace.ID
	agentWorkstreamID := s.callingSessionWorkstream(ctx)
	q.AgentWorkstreamID = agentWorkstreamID
	call := s.daemonCall
	if call == nil {
		call = daemon.Call
	}
	response, err := call(ctx, s.paths.Socket, *q)
	if err != nil {
		return toolOutput{}, fmt.Errorf("local Stickguy service unavailable: %w", err)
	}
	if !response.OK {
		return toolOutput{}, errors.New(response.Error)
	}
	reportedWorkstream := workspace.WorkstreamID
	if agentWorkstreamID != "" {
		reportedWorkstream = agentWorkstreamID
	}
	out := toolOutput{ProjectID: workspace.ProjectID, WorkspaceID: workspace.ID, WorkstreamID: reportedWorkstream}
	encoded, _ := json.Marshal(response.Data)
	_ = json.Unmarshal(encoded, &out)
	return out, nil
}

func (s *server) callingSessionWorkstream(ctx context.Context) string {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	if s.agentWorkstreamID != "" {
		return s.agentWorkstreamID
	}
	cwd := s.cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	call := s.daemonCall
	if call == nil && s.paths.Socket != "" {
		call = daemon.Call
	}
	resolved := sessionWorkstream(ctx, s.paths.Socket, cwd, call)
	if resolved != "" {
		s.agentWorkstreamID = resolved
	}
	return resolved
}

func (s *server) resolveWorkspace(requested string) (config.Workspace, error) {
	if requested != "" {
		for _, workspace := range s.cfg.Workspaces {
			if workspace.ID == requested {
				return workspace, nil
			}
		}
		return config.Workspace{}, errors.New("workspace_not_registered")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return config.Workspace{}, fmt.Errorf("resolve cwd: %w", err)
	}
	canonicalCWD, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return config.Workspace{}, fmt.Errorf("canonicalize cwd: %w", err)
	}
	var matches []config.Workspace
	for _, workspace := range s.cfg.Workspaces {
		root, err := filepath.EvalSymlinks(workspace.Root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, canonicalCWD)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			matches = append(matches, workspace)
		}
	}
	if len(matches) == 0 {
		return config.Workspace{}, errors.New("workspace_not_registered")
	}
	if len(matches) != 1 {
		return config.Workspace{}, errors.New("workspace_registration_ambiguous")
	}
	return matches[0], nil
}

func verification(values []verificationInput) []daemon.VerificationSummary {
	out := make([]daemon.VerificationSummary, len(values))
	for i, value := range values {
		out[i] = daemon.VerificationSummary{State: value.State, CheckKind: value.CheckKind, Label: value.Label, Summary: value.Summary, AffectedComponent: value.AffectedComponent, ObservedAt: value.ObservedAt, ManifestRevision: value.ManifestRevision}
	}
	return out
}
