//go:build darwin

package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/daemon"
	"github.com/khalidM3/overgent/internal/hosted"
	"github.com/khalidM3/overgent/internal/store"
)

// fakeBackend is one /v1 server. It records what reached it and with which
// credential, which is the whole question this file exists to answer: a
// profile holding two backends must never send one's events, heartbeats, or
// brief requests to the other.
type fakeBackend struct {
	server *httptest.Server
	token  string

	mu         sync.Mutex
	events     []string
	heartbeats []string
	briefs     []string
	reject     bool
}

func newFakeBackend(t *testing.T, token string) *fakeBackend {
	t.Helper()
	backend := &fakeBackend{token: token}
	backend.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(request.Body, 1<<20))
		if request.Header.Get("Authorization") != "Bearer "+backend.token {
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte(`{"error":{"code":"unauthorized","message":"wrong device credential"}}`))
			return
		}
		backend.mu.Lock()
		defer backend.mu.Unlock()
		if backend.reject {
			writer.WriteHeader(http.StatusUnauthorized)
			_, _ = writer.Write([]byte(`{"error":{"code":"unauthorized","message":"revoked"}}`))
			return
		}
		switch {
		case request.URL.Path == "/v1/events/batch":
			var decoded struct {
				Events []struct {
					EventID     string `json:"eventId"`
					WorkspaceID string `json:"workspaceId"`
				} `json:"events"`
			}
			_ = json.Unmarshal(body, &decoded)
			accepted := make([]string, 0, len(decoded.Events))
			for _, event := range decoded.Events {
				accepted = append(accepted, event.EventID)
				backend.events = append(backend.events, event.WorkspaceID)
			}
			writer.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(writer).Encode(map[string]any{"acceptedEventIds": accepted, "cursor": "cur_1"})
		case request.URL.Path == "/v1/presence/heartbeat":
			var decoded struct {
				WorkspaceID string `json:"workspaceId"`
			}
			_ = json.Unmarshal(body, &decoded)
			backend.heartbeats = append(backend.heartbeats, decoded.WorkspaceID)
			writer.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(request.URL.Path, "/briefs"):
			workstreamID := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/v1/workstreams/"), "/briefs")
			backend.briefs = append(backend.briefs, workstreamID)
			writer.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(writer).Encode(map[string]any{"briefId": "brf_fixture", "workstreamId": workstreamID, "items": []any{}})
		default:
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(backend.server.Close)
	return backend
}

func (b *fakeBackend) seen() ([]string, []string, []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.events...), append([]string(nil), b.heartbeats...), append([]string(nil), b.briefs...)
}

// onlyFrom reports whether every event a backend saw came from one workspace.
func onlyFrom(seen []string, workspaceID string) bool {
	for _, id := range seen {
		if id != workspaceID {
			return false
		}
	}
	return true
}

func (b *fakeBackend) refuse() {
	b.mu.Lock()
	b.reject = true
	b.mu.Unlock()
}

// twoBackendService builds a profile holding one Project per backend, with a
// distinct device credential for each, and the real hosted client in front of
// both. Only the credential distinguishes them, which is exactly the mistake
// this arrangement is meant to catch.
func twoBackendService(t *testing.T, first, second *fakeBackend) (*Service, *store.Store) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := config.Single(first.server.URL, "dev_first", []config.Workspace{{ID: "wsp_first", ProjectID: "prj_first", WorkstreamID: "wrk_first", MemberID: "mem_first", SessionID: "ses_first", Root: t.TempDir()}})
	cfg, secondBackend, err := cfg.UpsertBackend(second.server.URL, "dev_second")
	if err != nil {
		t.Fatal(err)
	}
	cfg = cfg.BindProject("prj_second", secondBackend.ID)
	cfg.Workspaces = append(cfg.Workspaces, config.Workspace{ID: "wsp_second", ProjectID: "prj_second", WorkstreamID: "wrk_second", MemberID: "mem_second", SessionID: "ses_second", Root: t.TempDir()})
	tokens := map[string]string{"dev_first": first.token, "dev_second": second.token}
	ctx := context.Background()
	for _, workspace := range cfg.Workspaces {
		backend, _ := cfg.BackendForWorkspace(workspace)
		if err = db.UpsertWorkspace(ctx, store.Workspace{
			ID: workspace.ID, ProjectID: workspace.ProjectID, WorkstreamID: workspace.WorkstreamID,
			MemberID: workspace.MemberID, DeviceID: backend.DeviceID, SessionID: workspace.SessionID,
			Root: workspace.Root, Baseline: strings.Repeat("a", 40), Fingerprint: "opaque", BackendID: backend.ID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	service := &Service{
		store: db, cfg: cfg,
		newSender: func(_ context.Context, backend config.Backend) (Sender, error) {
			client, clientErr := hosted.New(backend.APIBaseURL, tokens[backend.DeviceID])
			if clientErr != nil {
				return nil, clientErr
			}
			return hostedSender{client: client}, nil
		},
	}
	return service, db
}

// The whole point of ADR-074: two Projects on one profile publish to two
// servers, each with its own credential. A single service-wide client would
// send both to one of them, and the wrong credential would be rejected there.
func TestEventsFromTwoWorkspacesReachTheirOwnBackend(t *testing.T) {
	ctx := context.Background()
	first, second := newFakeBackend(t, "token-first"), newFakeBackend(t, "token-second")
	service, db := twoBackendService(t, first, second)
	for _, workspace := range []string{"wsp_first", "wsp_second"} {
		if err := db.EnqueueEvent(ctx, workspace, newID("evt_"), "manual", "activity.reported", map[string]any{"kind": "decision", "summary": workspace}); err != nil {
			t.Fatal(err)
		}
	}
	if !service.flush(ctx) {
		t.Fatalf("flush reported failure: %q", service.publishError())
	}
	firstEvents, _, _ := first.seen()
	secondEvents, _, _ := second.seen()
	// Each workspace also carries its registration event, so what matters is
	// that every event a backend saw belongs to its own workspace.
	if len(firstEvents) == 0 || !onlyFrom(firstEvents, "wsp_first") {
		t.Fatalf("first backend received %v", firstEvents)
	}
	if len(secondEvents) == 0 || !onlyFrom(secondEvents, "wsp_second") {
		t.Fatalf("second backend received %v", secondEvents)
	}
	pending, err := db.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("events left pending after both backends accepted: %d", len(pending))
	}
}

// A 401 is a recoverable state, not a verdict on the events, so the refused
// workspace's window stays pending. What must not happen is the other
// backend's queue waiting behind it: before this lane, one unreachable server
// stopped the whole profile publishing.
func TestOneRejectingBackendDoesNotStopTheOther(t *testing.T) {
	ctx := context.Background()
	first, second := newFakeBackend(t, "token-first"), newFakeBackend(t, "token-second")
	service, db := twoBackendService(t, first, second)
	first.refuse()
	for _, workspace := range []string{"wsp_first", "wsp_second"} {
		if err := db.EnqueueEvent(ctx, workspace, newID("evt_"), "manual", "activity.reported", map[string]any{"kind": "decision", "summary": workspace}); err != nil {
			t.Fatal(err)
		}
	}
	if service.flush(ctx) {
		t.Fatal("flush reported success while one backend was rejecting")
	}
	secondEvents, _, _ := second.seen()
	if len(secondEvents) == 0 || !onlyFrom(secondEvents, "wsp_second") {
		t.Fatalf("the healthy backend received %v", secondEvents)
	}
	pending, err := db.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) == 0 {
		t.Fatal("the refused workspace's events were dropped rather than left pending")
	}
	for _, event := range pending {
		if event.WorkspaceID != "wsp_first" {
			t.Fatalf("the healthy backend's events stayed pending: %+v", pending)
		}
	}
}

// Presence is per workspace on the wire already; only the client differs. A
// heartbeat sent to the wrong backend is a workspace that server has never
// heard of, reported as live.
func TestHeartbeatsGoToTheBackendTheWorkspaceBelongsTo(t *testing.T) {
	ctx := context.Background()
	first, second := newFakeBackend(t, "token-first"), newFakeBackend(t, "token-second")
	service, _ := twoBackendService(t, first, second)
	service.sendHeartbeats(ctx)
	_, firstBeats, _ := first.seen()
	_, secondBeats, _ := second.seen()
	if len(firstBeats) != 1 || firstBeats[0] != "wsp_first" {
		t.Fatalf("first backend heartbeats = %v", firstBeats)
	}
	if len(secondBeats) != 1 || secondBeats[0] != "wsp_second" {
		t.Fatalf("second backend heartbeats = %v", secondBeats)
	}
}

// A coordination brief for a session on one backend must never be requested
// from the other: the answer would be about a Project the caller is not in.
func TestLifecycleBriefsAreFetchedFromTheProjectsOwnBackend(t *testing.T) {
	ctx := context.Background()
	first, second := newFakeBackend(t, "token-first"), newFakeBackend(t, "token-second")
	service, _ := twoBackendService(t, first, second)
	response := service.handle(ctx, daemon.Request{
		Method: "begin_work", WorkspaceID: "wsp_second", IdempotencyKey: "begin_1",
		Title: "Second Project work", IntendedOutcome: "Prove the brief goes to the right server",
	})
	if !response.OK {
		t.Fatalf("begin_work response = %#v", response)
	}
	_, _, firstBriefs := first.seen()
	_, _, secondBriefs := second.seen()
	if len(firstBriefs) != 0 {
		t.Fatalf("a brief for the second Project reached the first backend: %v", firstBriefs)
	}
	if len(secondBriefs) != 1 || secondBriefs[0] != "wrk_second" {
		t.Fatalf("second backend briefs = %v", secondBriefs)
	}
}

// Health reports one credential state per backend. A single value could only
// ever describe one of them, and the desktop would show a revoked team Project
// as the state of the local Project beside it.
func TestHealthReportsEveryBackendSeparately(t *testing.T) {
	ctx := context.Background()
	first, second := newFakeBackend(t, "token-first"), newFakeBackend(t, "token-second")
	service, _ := twoBackendService(t, first, second)
	service.sendHeartbeats(ctx)
	response := service.handle(ctx, daemon.Request{Method: "health"})
	data, ok := response.Data.(map[string]any)
	if !response.OK || !ok {
		t.Fatalf("health response = %#v", response)
	}
	backends, ok := data["backends"].([]map[string]any)
	if !ok || len(backends) != 2 {
		t.Fatalf("health backends = %#v", data["backends"])
	}
	origins := map[string]bool{}
	for _, backend := range backends {
		origins[backend["apiBaseUrl"].(string)] = true
		if backend["credential"] != string(hosted.CredentialOK) {
			t.Fatalf("backend %v credential = %v", backend["id"], backend["credential"])
		}
	}
	if !origins[first.server.URL] || !origins[second.server.URL] {
		t.Fatalf("health named %v, want both backends", origins)
	}
}
