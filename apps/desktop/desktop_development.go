//go:build darwin && !production

package main

import (
	"context"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/khalidM3/overgent/internal/config"
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

// desktopTeamActivationOrigin names the origin that serves a Project's
// dashboard and redeems its activation ticket.
//
// A dashboard ticket is minted by one backend and is only redeemable there, so
// this has to follow the Project's own backend. The previous version ignored
// its argument and always answered with Vite, which meant a Project on a remote
// server minted a ticket on that server and then presented it to the local
// development backend, which had never issued it: the member saw "This
// dashboard link is no longer valid" with nothing actually expired.
//
// Vite is the answer only for the development backend itself, because there the
// dev server is what serves the dashboard and proxies /api to it.
func desktopTeamActivationOrigin(apiBaseURL string) string {
	origin := strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if origin == "" || config.IsLoopbackOrigin(origin) {
		return developmentOrigin("OVERGENT_DASHBOARD_ORIGIN", "http://127.0.0.1:5173/api")
	}
	return origin
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
	return openNewestProject(ctx, window, desktopConfigRoot())
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
