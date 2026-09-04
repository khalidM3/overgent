//go:build darwin && !production

package main

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/khalidM3/overgent/internal/activation"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/credential"
	"github.com/khalidM3/overgent/internal/hosted"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const desktopDevelopment = true

func desktopProductName() string { return "Overgent Dev" }
func desktopMenuLabel() string   { return "Overgent development" }
func desktopStartURL() string    { return "/?desktop=onboarding" }
func desktopURLScheme() string   { return "overgent-dev" }

// desktopAPIBaseURL is the development harness's origin. Unlike production it
// does not prefer the profile's stored origin: `pnpm dev` chooses the origin
// per run through this variable, and a stored one silently winning is how a
// developer ends up debugging the wrong deployment.
func desktopAPIBaseURL() string {
	return developmentOrigin("OVERGENT_API_ORIGIN", "http://127.0.0.1:3211")
}

// desktopActivationBaseURL is Vite in development: one origin serving the
// dashboard and proxying /api to the backend. A local-mode profile uses the
// app's own equivalent instead, so the same flow works with no dev server.
func desktopActivationBaseURL() string {
	if origin := localDashboardOrigin(); origin != "" {
		return origin
	}
	return developmentOrigin("OVERGENT_DASHBOARD_ORIGIN", "http://127.0.0.1:5173/api")
}
func desktopCLIBinary() string { return os.Getenv("OVERGENT_CLI_BINARY") }
func desktopConfigRoot() string {
	value := strings.TrimSpace(os.Getenv("OVERGENT_CONFIG_ROOT"))
	if value != "" && filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	root, _ := config.DefaultRoot()
	return root
}

func openLocalProject(ctx context.Context, window *application.WebviewWindow) error {
	root := desktopConfigRoot()
	paths, err := config.Resolve(root)
	if err != nil {
		return err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}
	if len(cfg.Workspaces) == 0 || cfg.DeviceID == "" || cfg.APIBaseURL == "" {
		return errors.New("local live view requires an enrolled Project in the default development profile")
	}
	projectID := cfg.Workspaces[len(cfg.Workspaces)-1].ProjectID
	if !developmentOriginAllowed(cfg.APIBaseURL) {
		return errors.New("development desktop requires loopback HTTP or an HTTPS shared-development API origin")
	}
	token, err := credential.Get(ctx, cfg.DeviceID)
	if err != nil {
		return err
	}
	client, err := hosted.New(cfg.APIBaseURL, token)
	if err != nil {
		return err
	}
	ticket, err := client.CreateDashboardTicket(ctx, projectID)
	if err != nil {
		return err
	}
	handoff, err := activation.Start(desktopActivationBaseURL(), ticket.Ticket)
	if err != nil {
		return err
	}
	window.SetURL(handoff.URL())
	return handoff.Wait(ctx)
}

func developmentOriginAllowed(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	if parsed.Scheme != "http" {
		return false
	}
	host := parsed.Hostname()
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func developmentOrigin(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if !developmentOriginAllowed(value) {
		return fallback
	}
	return strings.TrimRight(value, "/")
}
