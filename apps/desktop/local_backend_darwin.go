//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/khalidM3/overgent/internal/activation"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/credential"
	"github.com/khalidM3/overgent/internal/daemon"
	"github.com/khalidM3/overgent/internal/hosted"
	"github.com/khalidM3/overgent/internal/localbackend"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// storedAPIBaseURL is the team server this profile most recently enrolled
// against, or empty when it has none. Read failures are empty too: a profile
// whose config cannot be read has no stored answer, and the build default is
// the honest fallback.
//
// It is the default offered when a member creates another team Project, which
// is what makes self-hosting work from a stock build (Lane 05): the origin is
// entered once and every later team Project starts from it. Local backends are
// skipped - loopback is never a sensible default for a Project meant to have
// remote members - and the member can still type a different server.
func storedAPIBaseURL(configRoot string) string {
	if strings.TrimSpace(configRoot) == "" {
		return ""
	}
	paths, err := config.Resolve(configRoot)
	if err != nil {
		return ""
	}
	loaded, err := config.Load(paths)
	if err != nil {
		return ""
	}
	origin := ""
	for _, backend := range loaded.Backends {
		if backend.Kind == config.KindTeam {
			origin = backend.APIBaseURL
		}
	}
	return strings.TrimRight(strings.TrimSpace(origin), "/")
}

// The app serves one dashboard origin for local mode, for its whole lifetime.
// It is package state because both the onboarding service and the activation
// helpers need the same origin, and starting a second one would put the
// activation cookie on an origin the dashboard is not served from.
var (
	dashboardOriginOnce sync.Once
	sharedOrigin        *dashboardOrigin
)

// ensureDashboardOrigin starts the local dashboard origin once, on demand.
func ensureDashboardOrigin(assets fs.FS) *dashboardOrigin {
	dashboardOriginOnce.Do(func() {
		started, err := startDashboardOrigin(assets)
		if err != nil {
			slog.Warn("local dashboard origin unavailable", "error", err)
			return
		}
		sharedOrigin = started
	})
	return sharedOrigin
}

// activationOriginFor is the origin one Project's dashboard is opened against.
//
// It is per Project, not per profile: a team Project is served by its own
// deployment, and a local Project is served by this app, which is the local
// equivalent of Vite in development and of Vercel in production. A profile
// holding both asks this question once per Project rather than once per Mac.
//
// The local branch also brings the backend up and points the proxy at it,
// because the proxy's target is only known once the service has started the
// backend - and on a relaunch nothing else would have done it.
func activationOriginFor(ctx context.Context, paths config.Paths, backend config.Backend) (string, error) {
	if backend.Kind != config.KindLocal {
		return desktopTeamActivationOrigin(backend.APIBaseURL), nil
	}
	endpoint, err := ensureLocalBackend(ctx, paths)
	if err != nil {
		return "", err
	}
	origin := ensureDashboardOrigin(dashboardAssets())
	if origin == nil {
		return "", errors.New("the local dashboard could not be served")
	}
	if err = origin.SetBackend(endpoint.SiteOrigin); err != nil {
		return "", err
	}
	return origin.Origin(), nil
}

// isLoopbackOrigin asks the same question the service and the CLI ask, through
// the same function, so the three cannot disagree about which Projects are
// local.
func isLoopbackOrigin(origin string) bool { return localbackend.IsLoopbackOrigin(origin) }

// bundledBackendArtifacts is where a production app bundle carries the backend
// binary and the release-time deploy payload.
func bundledBackendArtifacts() (string, string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	directory := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Resources", "backend"))
	binary := filepath.Join(directory, "convex-local-backend")
	bundle := filepath.Join(directory, "backend-push.json")
	for _, path := range []string{binary, bundle} {
		info, statErr := os.Stat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			return "", "", errors.New("this build does not carry a bundled backend")
		}
	}
	return binary, bundle, nil
}

// recordBundledBackend writes the bundled artifact paths into the profile on
// every launch, so the service finds them again after an app update moved them.
// It is intentionally quiet when there is nothing to record: a development
// build without a bundled backend is a normal state, not a fault.
func recordBundledBackend(configRoot string) {
	binary, bundle, err := bundledBackendArtifacts()
	if err != nil {
		return
	}
	if err = localbackend.Install(configRoot, binary, bundle); err != nil {
		slog.Warn("record bundled backend paths", "error", err)
	}
}

// ensureLocalBackend asks the running service to start the backend and returns
// where it is. The desktop never spawns the backend itself: one process has to
// own its lifetime, and that process is the service, which outlives the window.
func ensureLocalBackend(ctx context.Context, paths config.Paths) (localbackend.Endpoint, error) {
	response, err := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "backend_ensure"})
	if err != nil {
		return localbackend.Endpoint{}, errors.New("the Overgent background service is not running yet")
	}
	if !response.OK {
		return localbackend.Endpoint{}, errors.New(response.Error)
	}
	encoded, err := json.Marshal(response.Data)
	if err != nil {
		return localbackend.Endpoint{}, err
	}
	var endpoint localbackend.Endpoint
	if err = json.Unmarshal(encoded, &endpoint); err != nil {
		return localbackend.Endpoint{}, err
	}
	if endpoint.SiteOrigin == "" {
		return localbackend.Endpoint{}, errors.New("the local backend reported no address")
	}
	return endpoint, nil
}

// BackendStatus is the backend half of the menu's service health.
type BackendStatus struct {
	Present    bool   `json:"present"`
	Running    bool   `json:"running"`
	Port       int    `json:"port,omitempty"`
	SitePort   int    `json:"sitePort,omitempty"`
	Version    string `json:"version,omitempty"`
	LastError  string `json:"lastError,omitempty"`
	SizeOnDisk int64  `json:"sizeOnDisk,omitempty"`
}

// dashboardAssets is the embedded dashboard bundle the local origin serves. It
// is the same build the Wails asset handler serves, not a second copy.
func dashboardAssets() fs.FS {
	assets, err := fs.Sub(embeddedAssets, "frontend/embed/app")
	if err != nil {
		return nil
	}
	return assets
}

// backendStatus reports the bundled backend for the onboarding screen and the
// menu. It reads the service's health rather than probing the backend itself,
// because the service is the process that knows.
func (service *OnboardingService) backendStatus() BackendStatus {
	status := BackendStatus{Present: service.localAvailable}
	if !status.Present {
		return status
	}
	paths, err := config.Resolve(service.configRoot)
	if err != nil {
		return status
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	response, err := daemon.Call(ctx, paths.Socket, daemon.Request{Method: "backend_status"})
	if err != nil || !response.OK {
		return status
	}
	encoded, err := json.Marshal(response.Data)
	if err != nil {
		return status
	}
	var reported localbackend.Status
	if err = json.Unmarshal(encoded, &reported); err != nil {
		return status
	}
	status.Running = reported.Running
	status.Port = reported.Port
	status.SitePort = reported.SitePort
	status.Version = reported.Version
	status.LastError = reported.LastError
	status.SizeOnDisk = reported.DatabaseBytes
	return status
}

// openNewestProject opens the live view for the Project this Mac most recently
// added, in the window the app already has. It is the menu's "open" action and
// is shared by both build profiles, because the only thing that differs
// between them is which origin serves a team Project's dashboard.
func openNewestProject(ctx context.Context, window *application.WebviewWindow, configRoot string) error {
	paths, err := config.Resolve(configRoot)
	if err != nil {
		return err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}
	if len(cfg.Workspaces) == 0 {
		return errors.New("live view requires an enrolled Project")
	}
	projectID := cfg.Workspaces[len(cfg.Workspaces)-1].ProjectID
	url, err := liveProjectURL(ctx, paths, cfg, projectID)
	if err != nil {
		return err
	}
	handoff, err := activation.Start(url.origin, url.ticket)
	if err != nil {
		return err
	}
	window.SetURL(handoff.URL())
	return handoff.Wait(ctx)
}

type liveProject struct{ origin, ticket string }

// liveProjectURL mints a one-time dashboard session for one Project against
// the backend that Project lives on, and names the origin its dashboard is
// served from. Both are per Project after ADR-074: the credential, the server,
// and the dashboard all follow the Project rather than the profile.
func liveProjectURL(ctx context.Context, paths config.Paths, cfg config.Config, projectID string) (liveProject, error) {
	backend, bound := cfg.BackendForProject(projectID)
	if !bound || backend.DeviceID == "" {
		return liveProject{}, errors.New("Project is not enrolled on this device")
	}
	token, err := credential.Get(ctx, backend.DeviceID)
	if err != nil {
		return liveProject{}, err
	}
	client, err := hosted.New(backend.APIBaseURL, token)
	if err != nil {
		return liveProject{}, err
	}
	ticket, err := client.CreateDashboardTicket(ctx, projectID)
	if err != nil {
		return liveProject{}, err
	}
	origin, err := activationOriginFor(ctx, paths, backend)
	if err != nil {
		return liveProject{}, err
	}
	return liveProject{origin: origin, ticket: ticket.Ticket}, nil
}
