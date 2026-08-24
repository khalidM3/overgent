//go:build darwin && !production

package main

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/stickguy/stickguy/internal/activation"
	"github.com/stickguy/stickguy/internal/config"
	"github.com/stickguy/stickguy/internal/credential"
	"github.com/stickguy/stickguy/internal/hosted"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const desktopDevelopment = true

func desktopProductName() string { return "Stickguy Dev" }
func desktopMenuLabel() string   { return "Stickguy development" }

func desktopStartURL() string {
	value := os.Getenv("STICKGUY_DESKTOP_DEV_URL")
	if value == "" {
		value = "http://127.0.0.1:5173/?desktop=preview"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Host == "" || parsed.Fragment != "" {
		return "/?desktop=preview"
	}
	host := parsed.Hostname()
	if host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return "/?desktop=preview"
	}
	return parsed.String()
}

func openLocalProject(ctx context.Context, window *application.WebviewWindow) error {
	root, err := config.DefaultRoot()
	if err != nil {
		return err
	}
	paths, err := config.Resolve(root)
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
		return errors.New("local live view requires one enrolled Project in the default development profile")
	}
	var projectID string
	for value := range projects {
		projectID = value
	}
	if !strings.HasPrefix(cfg.APIBaseURL, "http://127.0.0.1:") && !strings.HasPrefix(cfg.APIBaseURL, "http://localhost:") {
		return errors.New("development desktop refuses a non-loopback API origin")
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
	handoff, err := activation.Start(cfg.APIBaseURL, ticket.Ticket)
	if err != nil {
		return err
	}
	window.SetURL(handoff.URL())
	return handoff.Wait(ctx)
}
