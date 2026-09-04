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

	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/daemon"
	"github.com/khalidM3/overgent/internal/localbackend"
)

// storedAPIBaseURL is the backend origin this profile was enrolled against, or
// empty when it has not enrolled yet. Read failures are empty too: a profile
// whose config cannot be read has no stored answer, and the build default is
// the honest fallback.
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
	return strings.TrimRight(strings.TrimSpace(loaded.APIBaseURL), "/")
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

// localDashboardOrigin is the origin to activate against for a local Project,
// or empty when this profile is not in local mode.
func localDashboardOrigin() string {
	if sharedOrigin == nil {
		return ""
	}
	if !localbackend.Configured(desktopConfigRoot()) {
		return ""
	}
	stored := storedAPIBaseURL(desktopConfigRoot())
	if stored != "" && !isLoopbackOrigin(stored) {
		// A team-mode Project on this profile is served by its own deployment.
		return ""
	}
	return sharedOrigin.Origin()
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
