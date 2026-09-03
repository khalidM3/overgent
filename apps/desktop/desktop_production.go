//go:build darwin && production

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/khalidM3/overgent/internal/activation"
	"github.com/khalidM3/overgent/internal/config"
	"github.com/khalidM3/overgent/internal/credential"
	"github.com/khalidM3/overgent/internal/hosted"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const desktopDevelopment = false

func desktopProductName() string { return "Overgent" }
func desktopMenuLabel() string   { return "Overgent beta" }
func desktopStartURL() string    { return "/?desktop=onboarding" }
func desktopURLScheme() string   { return "overgent" }

// apiBaseURL is the hosted origin a production build talks to. Releases keep the
// default; a private build for a closed test overrides it with
// -X main.apiBaseURL=... so the app does not have to be edited to point at a
// different deployment. Activation rejects anything that is not HTTPS.
var apiBaseURL = "https://api.overgent.com"

func desktopAPIBaseURL() string        { return apiBaseURL }
func desktopActivationBaseURL() string { return desktopAPIBaseURL() }
func desktopCLIBinary() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	bundled := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Resources", "overgent"))
	home, err := os.UserHomeDir()
	if err != nil {
		return bundled
	}
	directory := filepath.Join(home, ".local", "bin")
	installed := filepath.Join(directory, "overgent")
	if info, statErr := os.Stat(installed); statErr == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
		return installed
	}
	if err = os.MkdirAll(directory, 0o700); err != nil {
		return bundled
	}
	input, err := os.Open(bundled)
	if err != nil {
		return bundled
	}
	defer input.Close()
	temporary, err := os.CreateTemp(directory, ".overgent-desktop-install-*")
	if err != nil {
		return bundled
	}
	if err = temporary.Chmod(0o755); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporary.Name())
		return bundled
	}
	_, copyErr := io.Copy(temporary, input)
	closeErr := temporary.Close()
	if copyErr != nil || closeErr != nil || os.Rename(temporary.Name(), installed) != nil {
		_ = os.Remove(temporary.Name())
		return bundled
	}
	return installed
}
func desktopConfigRoot() string { root, _ := config.DefaultRoot(); return root }
func openLocalProject(ctx context.Context, window *application.WebviewWindow) error {
	paths, err := config.Resolve(desktopConfigRoot())
	if err != nil {
		return err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}
	if len(cfg.Workspaces) == 0 || cfg.DeviceID == "" || cfg.APIBaseURL == "" {
		return errors.New("live view requires an enrolled Project")
	}
	projectID := cfg.Workspaces[len(cfg.Workspaces)-1].ProjectID
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
