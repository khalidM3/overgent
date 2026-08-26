//go:build darwin && production

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/stickguy/stickguy/internal/activation"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/credential"
	"github.com/stickguy/stickguy/internal/hosted"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const desktopDevelopment = false

func desktopProductName() string       { return "Stickguy" }
func desktopMenuLabel() string         { return "Stickguy beta" }
func desktopStartURL() string          { return "/?desktop=onboarding" }
func desktopAPIBaseURL() string        { return "https://api.stickguy.dev" }
func desktopActivationBaseURL() string { return desktopAPIBaseURL() }
func desktopCLIBinary() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Resources", "stickguy"))
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
	projects := map[string]struct{}{}
	for _, workspace := range cfg.Workspaces {
		projects[workspace.ProjectID] = struct{}{}
	}
	if len(projects) != 1 || cfg.DeviceID == "" || cfg.APIBaseURL == "" {
		return errors.New("live view requires one enrolled Project")
	}
	var projectID string
	for value := range projects {
		projectID = value
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
