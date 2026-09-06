//go:build darwin

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/localbackend"
)

func TestStoredAPIBaseURLPrefersTheProfileAndCanonicalizes(t *testing.T) {
	root := t.TempDir()
	if got := storedAPIBaseURL(root); got != "" {
		t.Fatalf("an unenrolled profile reported %q", got)
	}
	paths, err := config.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	// This is what makes a stock build talk to a self-hosted server: the origin
	// a team Project was created against is the default every later team
	// Project starts from.
	if err = config.Save(paths, config.Single("https://convex.example.com/", "dev_test", nil)); err != nil {
		t.Fatal(err)
	}
	if got := storedAPIBaseURL(root); got != "https://convex.example.com" {
		t.Fatalf("stored origin=%q", got)
	}
	// A loopback backend is never the default for a Project meant to have
	// remote members, so a profile holding both still reports the team server.
	both, backendErr := config.Load(paths)
	if backendErr != nil {
		t.Fatal(backendErr)
	}
	both, _, backendErr = both.UpsertBackend("http://127.0.0.1:43103", "dev_local")
	if backendErr != nil {
		t.Fatal(backendErr)
	}
	if err = config.Save(paths, both); err != nil {
		t.Fatal(err)
	}
	if got := storedAPIBaseURL(root); got != "https://convex.example.com" {
		t.Fatalf("a local backend became the default team server: %q", got)
	}
	if got := storedAPIBaseURL(""); got != "" {
		t.Fatalf("an empty root reported %q", got)
	}
}

func TestIsLoopbackOriginSeparatesLocalFromTeam(t *testing.T) {
	for _, local := range []string{"http://127.0.0.1:43103", "http://localhost:3211", "http://[::1]:3211"} {
		if !isLoopbackOrigin(local) {
			t.Fatalf("%q was not recognized as local", local)
		}
	}
	for _, team := range []string{"https://api.overgent.com", "http://example.com", "", "https://127.0.0.1"} {
		if isLoopbackOrigin(team) {
			t.Fatalf("%q was treated as local", team)
		}
	}
}

func TestBundledBackendArtifactsRequireBothFiles(t *testing.T) {
	// A build with only half the artifacts must report "no bundled backend"
	// rather than recording a path the service will later fail to start.
	root := t.TempDir()
	resources := filepath.Join(root, "Contents", "Resources", "backend")
	if err := os.MkdirAll(resources, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resources, "convex-local-backend"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := localbackend.Install(t.TempDir(), filepath.Join(resources, "convex-local-backend"), filepath.Join(resources, "backend-push.json")); err == nil {
		t.Fatal("install accepted a missing deploy payload")
	}
}

// The local dashboard origin is the third implementation of one arrangement -
// Vercel hosted, Vite in development, this in local mode - so the tests are
// about the two jobs it shares with them: same-origin assets, and a /api/v1
// forward whose redirect stays on this origin.
func TestLocalDashboardOriginServesAssetsAndForwardsTheAPI(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/dashboard-activations" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.Header().Set("Set-Cookie", "overgent_session=abc; Path=/; HttpOnly; SameSite=Strict")
		// What the backend actually answers over loopback: a redirect to the
		// Vite dev server, which does not exist in an installed app.
		writer.Header().Set("Location", "http://127.0.0.1:5173/?live=1")
		writer.WriteHeader(http.StatusSeeOther)
	}))
	defer backend.Close()

	assets := fstest.MapFS{"index.html": {Data: []byte("<html>dashboard</html>")}}
	origin, err := startDashboardOrigin(assets)
	if err != nil {
		t.Fatal(err)
	}
	defer origin.Close()
	if !strings.HasPrefix(origin.Origin(), "http://127.0.0.1:") {
		t.Fatalf("the dashboard origin is not loopback: %s", origin.Origin())
	}
	if err = origin.SetBackend(backend.URL); err != nil {
		t.Fatal(err)
	}

	page, err := http.Get(origin.Origin() + "/?live=1")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	body, _ := io.ReadAll(page.Body)
	if !strings.Contains(string(body), "dashboard") {
		t.Fatalf("the dashboard was not served: %q", body)
	}

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	activation, err := client.Post(origin.Origin()+"/api/v1/dashboard-activations", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer activation.Body.Close()
	if activation.StatusCode != http.StatusSeeOther {
		t.Fatalf("activation status=%d", activation.StatusCode)
	}
	// The session cookie has to reach the browser, and the redirect has to stay
	// on the origin that set it, or the dashboard loads without a session.
	if !strings.Contains(activation.Header.Get("Set-Cookie"), "overgent_session=") {
		t.Fatal("the activation cookie was dropped")
	}
	if location := activation.Header.Get("Location"); location != "/?live=1" {
		t.Fatalf("the redirect left this origin: %q", location)
	}
}

func TestLocalDashboardOriginRefusesANonLoopbackBackend(t *testing.T) {
	origin, err := startDashboardOrigin(fstest.MapFS{"index.html": {Data: []byte("x")}})
	if err != nil {
		t.Fatal(err)
	}
	defer origin.Close()
	for _, refused := range []string{"https://api.overgent.com", "http://example.com", "http://127.0.0.1:3211/v1", ""} {
		if err = origin.SetBackend(refused); err == nil {
			t.Fatalf("accepted %q as a local backend", refused)
		}
	}
	// A proxy with no backend answers honestly rather than hanging.
	response, err := http.Get(origin.Origin() + "/api/v1/dashboard/session")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status without a backend=%d", response.StatusCode)
	}
}

func TestLocalDashboardLocationKeepsHostedAndLoopbackRedirectsUsable(t *testing.T) {
	for input, want := range map[string]string{
		"http://127.0.0.1:5173/?live=1":             "/?live=1",
		"/dashboard?live=1":                         "/dashboard?live=1",
		"https://api.overgent.com/dashboard?live=1": "/?live=1",
		"":            "/?live=1",
		"::not a url": "/?live=1",
	} {
		if got := localDashboardLocation(input); got != want {
			t.Fatalf("localDashboardLocation(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestCreateLocalProjectRefusesWithoutABundledBackend(t *testing.T) {
	service := &OnboardingService{configRoot: t.TempDir(), localAvailable: false}
	_, err := service.CreateLocalProject(EnrollmentRequest{RepositoryRoot: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "does not carry a backend") {
		t.Fatalf("a build with no backend accepted a local Project: %v", err)
	}
}

// A Project binds to its own backend (ADR-074), so a Mac with a team Project
// can still add a local one. What must not happen is the reverse of the old
// rule: silently reusing the team backend for a Project the member asked to
// keep on this Mac.
func TestOnboardingStateNamesEachProjectsBackend(t *testing.T) {
	root := t.TempDir()
	paths, err := config.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	team, local := t.TempDir(), t.TempDir()
	cfg := config.Single("https://api.overgent.com", "dev_team", []config.Workspace{{ID: "wsp_team", ProjectID: "prj_team", Root: team}})
	cfg, localBackend, err := cfg.UpsertBackend("http://127.0.0.1:43103", "dev_local")
	if err != nil {
		t.Fatal(err)
	}
	cfg = cfg.BindProject("prj_local", localBackend.ID)
	cfg.Workspaces = append(cfg.Workspaces, config.Workspace{ID: "wsp_local", ProjectID: "prj_local", Root: local})
	if err = config.Save(paths, cfg); err != nil {
		t.Fatal(err)
	}
	service := &OnboardingService{configRoot: root, apiBaseURL: "https://api.overgent.com"}
	state, err := service.State()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	origins := map[string]string{}
	for _, project := range state.Projects {
		kinds[project.ProjectID] = project.Kind
		origins[project.ProjectID] = project.APIBaseURL
	}
	if kinds["prj_local"] != config.KindLocal || kinds["prj_team"] != config.KindTeam {
		t.Fatalf("Project kinds = %v", kinds)
	}
	if origins["prj_team"] != "https://api.overgent.com" || origins["prj_local"] != "http://127.0.0.1:43103" {
		t.Fatalf("Project origins = %v", origins)
	}
	// The newest registration is the selected Project, and the recovery action
	// names its backend rather than the whole Mac.
	if state.BackendID != localBackend.ID {
		t.Fatalf("selected backend = %q, want the newest Project's", state.BackendID)
	}
}

func TestBackendMenuLineOnlyAppearsForALocalProfile(t *testing.T) {
	// A team-mode Mac has no backend of its own, so the line is absent rather
	// than present and saying nothing.
	if label := (ServiceStatus{Connected: true}).BackendLabel(); label != "" {
		t.Fatalf("a team profile showed %q", label)
	}
	running := ServiceStatus{Connected: true, Backend: BackendHealth{Present: true, Running: true, Port: 43103}}
	if label := running.BackendLabel(); !strings.Contains(label, "127.0.0.1:43103") {
		t.Fatalf("running label=%q", label)
	}
	stopped := ServiceStatus{Connected: true, Backend: BackendHealth{Present: true}}
	if label := stopped.BackendLabel(); label != "Backend: stopped" {
		t.Fatalf("stopped label=%q", label)
	}
	// A failed update is said in the member's words, not as a status code.
	failed := ServiceStatus{Connected: true, Backend: BackendHealth{Present: true, Running: true, LastError: "update needs data migration"}}
	if label := failed.BackendLabel(); label != "Backend: update needs data migration" {
		t.Fatalf("failed label=%q", label)
	}
}

func TestBackendHealthReadsTheServiceBlock(t *testing.T) {
	if health := backendHealth(nil); health.Present {
		t.Fatal("a health response with no backend block reported one")
	}
	health := backendHealth(map[string]any{"running": true, "port": float64(43103), "version": "precompiled-x", "idleSince": "2026-09-04T10:00:00Z"})
	if !health.Present || !health.Running || health.Port != 43103 || health.Version != "precompiled-x" {
		t.Fatalf("unexpected backend health: %+v", health)
	}
	if health.Since.IsZero() {
		t.Fatal("idleSince was not parsed")
	}
}

// The seam between the dashboard origin and internal/activation.
//
// activation.Start appends "/v1/dashboard-activations" to the origin it is
// given, so the origin handed to it has to already carry the API prefix this
// mux forwards on. It did not: the ticket was posted to "/v1/...", missed the
// proxy, and was answered by the SPA file server with 200 and index.html - so
// activation silently set no cookie and the dashboard reported that the browser
// had no session. The proxy itself was tested with the prefix spelled correctly,
// which is exactly why nothing caught it.
func TestActivationOriginReachesTheProxyNotTheAssets(t *testing.T) {
	var reached string
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		reached = request.URL.Path
		writer.Header().Set("Set-Cookie", "overgent_session=abc; Path=/; HttpOnly; SameSite=Strict")
		writer.Header().Set("Location", "/?live=1")
		writer.WriteHeader(http.StatusSeeOther)
	}))
	defer backend.Close()

	origin, err := startDashboardOrigin(fstest.MapFS{"index.html": {Data: []byte("<html>dashboard</html>")}})
	if err != nil {
		t.Fatal(err)
	}
	defer origin.Close()
	if err = origin.SetBackend(backend.URL); err != nil {
		t.Fatal(err)
	}

	// Exactly the concatenation internal/activation performs on the origin it is
	// handed by activationOriginFor.
	action := origin.ActivationOrigin() + "/v1/dashboard-activations"
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Post(action, "application/x-www-form-urlencoded", strings.NewReader("ticket=test"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("activation posted to %s was not forwarded: status=%d body=%q", action, response.StatusCode, body)
	}
	if reached != "/v1/dashboard-activations" {
		t.Fatalf("the backend saw %q, not the activation route", reached)
	}
	if !strings.Contains(response.Header.Get("Set-Cookie"), "overgent_session=") {
		t.Fatal("activation returned no session cookie")
	}
}
