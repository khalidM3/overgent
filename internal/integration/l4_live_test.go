package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stickguy/stickguy/internal/app"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/daemon"
	"github.com/stickguy/stickguy/internal/hosted"
	"github.com/stickguy/stickguy/internal/onboarding"
)

type memoryCredentials struct {
	mu     sync.Mutex
	values map[string]string
}

func (m *memoryCredentials) Put(_ context.Context, account, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[account] = secret
	return nil
}
func (m *memoryCredentials) Delete(_ context.Context, account string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, account)
	return nil
}
func (m *memoryCredentials) Get(account string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.values[account]
}

type controlledSender struct {
	client  *hosted.Client
	offline atomic.Bool
}

func (s *controlledSender) Send(ctx context.Context, _ string, batch []byte) error {
	if s.offline.Load() {
		return fmt.Errorf("synthetic outage")
	}
	ack, err := s.client.PublishBatch(ctx, batch)
	if err != nil {
		return err
	}
	if len(ack.AcceptedEventIDs) == 0 || ack.Cursor == "" {
		return fmt.Errorf("incomplete acknowledgement")
	}
	return nil
}
func (s *controlledSender) Heartbeat(ctx context.Context, workspaceID, state string) error {
	if s.offline.Load() {
		return fmt.Errorf("synthetic outage")
	}
	return s.client.Heartbeat(ctx, workspaceID, state)
}

func TestL4TwoDeviceGoToHostedVerticalSlice(t *testing.T) {
	apiBase := os.Getenv("STICKGUY_L4_SITE_URL")
	if apiBase == "" {
		t.Skip("set STICKGUY_L4_SITE_URL to an anonymous loopback deployment")
	}
	creds := &memoryCredentials{values: map[string]string{}}
	service := onboarding.Service{
		Client:   func(token string) (onboarding.API, error) { return hosted.New(apiBase, token) },
		Creds:    creds,
		Register: app.Register,
	}
	rootA, rootB := tempStateRoot(t, "a"), tempStateRoot(t, "b")
	repoA, repoB := repository(t, "a"), repository(t, "b")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	created, err := service.Create(ctx, onboarding.Options{ConfigRoot: rootA, RepositoryRoot: repoA, APIBaseURL: apiBase, ProjectLabel: "Synthetic L4 Project", DeviceLabel: "Synthetic Device A"})
	if err != nil {
		t.Fatal(err)
	}
	if created.JoinCode == "" || created.DashboardTicket == "" {
		t.Fatal("creator result omitted invite or dashboard activation ticket")
	}
	joined, err := service.Join(ctx, onboarding.Options{ConfigRoot: rootB, RepositoryRoot: repoB, APIBaseURL: apiBase, DeviceLabel: "Synthetic Device B"}, created.JoinCode)
	if err != nil {
		t.Fatal(err)
	}
	if joined.ProjectID != created.ProjectID || joined.DashboardTicket == "" {
		t.Fatalf("joined result=%#v created=%#v", joined, created)
	}
	clientA := mustClient(t, apiBase, creds.Get(created.DeviceID))
	clientB := mustClient(t, apiBase, creds.Get(joined.DeviceID))
	senderA := &controlledSender{client: clientA}
	senderB := &controlledSender{client: clientB}
	senderB.offline.Store(true)
	cancelA, doneA := startService(t, rootA, senderA)
	defer stopService(t, cancelA, doneA)
	cancelB, doneB := startService(t, rootB, senderB)
	defer stopService(t, cancelB, doneB)
	reportIntent(t, rootA, created.WorkspaceID, "Coordinate shared boundary", "Update the shared overlap path safely")
	reportIntent(t, rootB, joined.WorkspaceID, "Coordinate shared boundary", "Update the shared overlap path safely")
	write(t, repoA, "shared/overlap.ts")
	write(t, repoB, "shared/overlap.ts")
	forceScan(t, rootA)
	forceScan(t, rootB)
	waitDoctor(t, rootA, func(pending int) bool { return pending == 0 })
	waitDoctor(t, rootB, func(pending int) bool { return pending > 0 })
	if err := clientA.Heartbeat(ctx, created.WorkspaceID, "active"); err != nil {
		t.Fatal(err)
	}
	senderB.offline.Store(false)
	reconnectStarted := time.Now()
	waitDoctor(t, rootB, func(pending int) bool { return pending == 0 })
	if err := clientB.Heartbeat(ctx, joined.WorkspaceID, "active"); err != nil {
		t.Fatal(err)
	}
	var finding map[string]any
	wait(t, func() bool {
		page, pageErr := clientA.ProjectChanges(ctx, created.ProjectID)
		if pageErr != nil {
			return false
		}
		for _, item := range page.Items {
			if item["kind"] == "direct_collision" {
				finding = item
				return true
			}
		}
		return false
	})
	if elapsed := time.Since(reconnectStarted); elapsed >= 5*time.Second {
		t.Fatalf("overlap appeared after %s, expected under five seconds", elapsed)
	}
	if finding["confidenceBand"] != "deterministic" {
		t.Fatalf("finding overstated or missing deterministic fidelity: %#v", finding)
	}
	paths, _ := config.Resolve(rootA)
	if _, err := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "pause", WorkspaceID: created.WorkspaceID}); err != nil {
		t.Fatal(err)
	}
	write(t, repoA, "shared/paused-only.ts")
	forceScan(t, rootA)
	waitDoctor(t, rootA, func(pending int) bool { return pending > 0 })
	assertDashboardSession(t, apiBase, created.DashboardTicket, created.ProjectID, clientA, created.DeviceID)
}

func reportIntent(t *testing.T, root, workspaceID, title, outcome string) {
	t.Helper()
	paths, _ := config.Resolve(root)
	response, err := daemon.Call(context.Background(), paths.Socket, daemon.Request{Method: "intent", WorkspaceID: workspaceID, Title: title, IntendedOutcome: outcome})
	if err != nil || !response.OK {
		t.Fatalf("report intent: %#v %v", response, err)
	}
}

func assertDashboardSession(t *testing.T, apiBase, ticket, projectID string, deviceClient *hosted.Client, deviceID string) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	body := url.Values{"ticket": {ticket}}.Encode()
	request, _ := http.NewRequest(http.MethodPost, apiBase+"/v1/dashboard-activations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("dashboard exchange status=%d", response.StatusCode)
	}
	if location := response.Header.Get("Location"); strings.Contains(location, ticket) || !strings.Contains(location, "live=1") {
		t.Fatalf("unsafe dashboard redirect=%q", location)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == "stickguy_session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil || !sessionCookie.HttpOnly || !sessionCookie.Secure || sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe dashboard cookie: %#v", sessionCookie)
	}
	get := func(path string, out any) {
		req, _ := http.NewRequest(http.MethodGet, apiBase+path, nil)
		req.AddCookie(sessionCookie)
		res, callErr := client.Do(req)
		if callErr != nil {
			t.Fatal(callErr)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("dashboard GET %s status=%d", path, res.StatusCode)
		}
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	var session struct {
		Projects []struct {
			ID string `json:"id"`
		} `json:"projects"`
	}
	get("/v1/dashboard/session", &session)
	if len(session.Projects) != 1 || session.Projects[0].ID != projectID {
		t.Fatalf("dashboard Project scope=%#v", session)
	}
	var snapshot struct{ Workstreams, Devices, Findings []json.RawMessage }
	get("/v1/dashboard/projects/"+projectID, &snapshot)
	if len(snapshot.Workstreams) != 2 || len(snapshot.Devices) != 2 || len(snapshot.Findings) < 1 {
		t.Fatalf("dashboard snapshot workstreams=%d devices=%d findings=%d", len(snapshot.Workstreams), len(snapshot.Devices), len(snapshot.Findings))
	}
	if err := deviceClient.RevokeDevice(context.Background(), deviceID); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, apiBase+"/v1/dashboard/session", nil)
	req.AddCookie(sessionCookie)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked-device browser session status=%d", res.StatusCode)
	}
}

func mustClient(t *testing.T, base, token string) *hosted.Client {
	t.Helper()
	client, err := hosted.New(base, token)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func tempStateRoot(t *testing.T, suffix string) string {
	t.Helper()
	root, err := os.MkdirTemp("/private/tmp", "stickguy-l4-"+suffix+"-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}

func repository(t *testing.T, suffix string) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "fixture@example.invalid")
	git(t, root, "config", "user.name", "Fixture")
	git(t, root, "remote", "add", "origin", "https://example.invalid/stickguy/l4-shared.git")
	write(t, root, "baseline.txt")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "baseline "+suffix)
	return root
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func write(t *testing.T, root, relative string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("synthetic fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func startService(t *testing.T, root string, sender app.Sender) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, root, sender) }()
	paths, _ := config.Resolve(root)
	wait(t, func() bool {
		_, err := daemon.Call(context.Background(), paths.Socket, daemon.Request{Method: "health"})
		return err == nil
	})
	return cancel, done
}

func stopService(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("service stop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("service did not stop")
	}
}

func forceScan(t *testing.T, root string) {
	t.Helper()
	paths, _ := config.Resolve(root)
	response, err := daemon.Call(context.Background(), paths.Socket, daemon.Request{Method: "scan"})
	if err != nil || !response.OK {
		t.Fatalf("force scan: %#v %v", response, err)
	}
}

func waitDoctor(t *testing.T, root string, predicate func(int) bool) {
	t.Helper()
	paths, _ := config.Resolve(root)
	wait(t, func() bool {
		response, err := daemon.Call(context.Background(), paths.Socket, daemon.Request{Method: "doctor"})
		if err != nil || !response.OK {
			return false
		}
		data, ok := response.Data.(map[string]any)
		pending, okPending := data["pending"].(float64)
		return ok && okPending && predicate(int(pending))
	})
}

func wait(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition did not converge")
}
