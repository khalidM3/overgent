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
	Method, WorkspaceID                                string
	ProjectID, WorkstreamID, MemberID, SessionID, Root string
	// APIBaseURL and DeviceID name the backend a Project being registered
	// lives on, and the device identity this profile uses with it (ADR-074).
	// They travel together, and only workspace registration carries them: a
	// Project joined on a friend's server may be the first thing this profile
	// has ever heard of that server.
	APIBaseURL, DeviceID                                                                string
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
	// AgentCandidateRoots holds every workspace root a vendor reported, for
	// vendors that report more than one. Only the service knows which of them is
	// registered, so the selection happens there.
	AgentCandidateRoots []string
	// AgentSessionTitle is already-classified title text (ADR-042). It is never
	// raw vendor text: the adapter runs ClassifyCoordinationTitle before this
	// field is populated.
	AgentSessionTitle                         string
	AgentTranscriptPath, AgentVendorSessionID string
	SinceRevision                             int64
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

// defaultCallTimeout bounds a request from a caller that supplied no deadline
// of its own. Most CLI callers pass a plain context and want to fail rather
// than hang against a wedged service; a caller that knows its method is slower
// says so with its own deadline.
const defaultCallTimeout = 3 * time.Second

// requestReadTimeout bounds how long a connected client may take to send its
// request, and handlerTimeout bounds the handler and the reply that follows it.
// handlerTimeout has to exceed the slowest method the service offers, which is
// "backend_ensure" and its ten-second backend health budget.
const (
	requestReadTimeout = 5 * time.Second
	handlerTimeout     = 60 * time.Second
)

func Serve(ctx context.Context, socket string, h Handler) error { return serve(ctx, socket, h) }
func Call(ctx context.Context, socket string, req Request) (Response, error) {
	c, e := dial(ctx, socket)
	if e != nil {
		return Response{}, fmt.Errorf("connect service: %w", e)
	}
	defer c.Close()
	// The caller's deadline governs when it has one, in both directions. This
	// used to take the minimum of the caller's deadline and three seconds,
	// which made three seconds a ceiling no caller could raise - and some
	// methods legitimately take longer than that. "backend_ensure" is the one
	// that matters: it starts the loopback backend and is allowed a ten-second
	// health budget by the service, so a client capped at three could never see
	// it succeed. It reported a timeout while the service went on and brought
	// the backend up, and the desktop turned that into "the background service
	// is not running yet" on a service that was running fine.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(defaultCallTimeout)
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
	// Reading the request is bounded tightly, because that phase is waiting on
	// the client: one that connects and then says nothing must not hold the
	// connection open.
	_ = c.SetReadDeadline(time.Now().Add(requestReadTimeout))
	var q Request
	if json.NewDecoder(io.LimitReader(c, 128<<10)).Decode(&q) != nil {
		return
	}
	// Running the handler and writing its answer gets its own, larger budget.
	// One five-second deadline used to cover both phases, which silently capped
	// how long any method could take: "backend_ensure" starts the loopback
	// backend under a ten-second health budget, so its answer could never be
	// written and the caller saw a closed connection instead of the result of
	// the work the service had just done. The real bound on a slow method is
	// the client's own deadline; this only stops a dead peer from pinning the
	// connection forever.
	_ = c.SetDeadline(time.Now().Add(handlerTimeout))
	_ = json.NewEncoder(c).Encode(h(ctx, q))
}

type Lock interface{ Close() error }

func Acquire(path string) (Lock, error) { return acquire(path) }
