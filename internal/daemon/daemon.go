package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

type Request struct {
	Method, WorkspaceID                                                                 string
	ProjectID, WorkstreamID, MemberID, SessionID, Root                                  string
	Title, IntendedOutcome                                                              string
	ApproachSummary                                                                     string
	Components, Contracts, WaitingOn                                                    []string
	AnticipatedPaths, PlanItemIDs                                                       []string
	IdempotencyKey, Trigger, SinceCursor, CheckpointID, BriefID, Kind, Summary, Outcome string
	Revision, ManifestRevision, ApproximateTokenBudget                                  int64
	Discoveries, ConsideredItemIDs                                                      []string
	Verification                                                                        []VerificationSummary
	AgentVendor, AgentCWD, AgentWorkstreamID, AgentSessionAlias                         string
	AgentEvent, AgentStatus, AgentAction, AgentTool, AgentType, AgentSubagentAlias      string
	AgentPaths                                                                          []string
	AgentTranscriptPath, AgentVendorSessionID                                           string
	SinceRevision                                                                       int64
	// FocusSeconds bounds how long an agent session stays free of injected
	// coordination. Zero means the service default; the store caps the maximum.
	FocusSeconds int64
}
type AgentMessage struct{ Kind, Text string }
type VerificationSummary struct {
	State, CheckKind, Label, Summary, AffectedComponent, Source, ObservedAt string
	ManifestRevision                                                        int64
}
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}
type Handler func(context.Context, Request) Response

func Serve(ctx context.Context, socket string, h Handler) error { return serve(ctx, socket, h) }
func Call(ctx context.Context, socket string, req Request) (Response, error) {
	c, e := dial(ctx, socket)
	if e != nil {
		return Response{}, fmt.Errorf("connect service: %w", e)
	}
	defer c.Close()
	deadline := time.Now().Add(3 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = c.SetDeadline(deadline)
	if e = json.NewEncoder(c).Encode(req); e != nil {
		return Response{}, e
	}
	var r Response
	if e = json.NewDecoder(io.LimitReader(c, 1<<20)).Decode(&r); e != nil {
		return r, e
	}
	return r, nil
}
func serveConn(ctx context.Context, c net.Conn, h Handler) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	var q Request
	if json.NewDecoder(io.LimitReader(c, 128<<10)).Decode(&q) != nil {
		return
	}
	_ = json.NewEncoder(c).Encode(h(ctx, q))
}

type Lock interface{ Close() error }

func Acquire(path string) (Lock, error) { return acquire(path) }
