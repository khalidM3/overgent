//go:build darwin && production

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// desktopCLIBinary returns the Overgent CLI this app should bind agents to,
// installing the bundled one at a stable path so that the managed hook and MCP
// commands do not name a path inside an app bundle that a later build replaces.
//
// It refreshes an installed copy that differs from the bundled one. The previous
// version returned any existing file without looking at it, so once
// ~/.local/bin/overgent existed it was never updated again: every subsequent app
// update shipped a new app driving an old CLI, and the two disagreed about the
// wire format, the hook set, and eventually the product's own name.
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
	if sameFileContents(bundled, installed) {
		return installed
	}
	if err = os.MkdirAll(directory, 0o700); err != nil {
		return bundled
	}
	input, err := os.Open(bundled)
	if err != nil {
		// No bundled binary to install from. An existing installed copy is still
		// better than a path that does not exist.
		if info, statErr := os.Stat(installed); statErr == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return installed
		}
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
	// Rename over the existing path rather than removing it first, so a CLI a
	// hook is executing right now is replaced atomically instead of vanishing.
	if copyErr != nil || closeErr != nil || os.Rename(temporary.Name(), installed) != nil {
		_ = os.Remove(temporary.Name())
		return bundled
	}
	return installed
}

// sameFileContents reports whether the installed CLI is already the bundled one.
// Size is checked first because it separates two different builds without
// reading 25MB twice; the digest only runs for the rare same-size case.
func sameFileContents(bundled, installed string) bool {
	bundledInfo, err := os.Stat(bundled)
	if err != nil {
		return false
	}
	installedInfo, err := os.Stat(installed)
	if err != nil || !installedInfo.Mode().IsRegular() || installedInfo.Mode()&0o111 == 0 {
		return false
	}
	if bundledInfo.Size() != installedInfo.Size() {
		return false
	}
	// An unreadable file digests to "", and two unreadable files must not
	// compare equal: that would report a missing binary as already installed.
	digest := fileDigest(bundled)
	return digest != "" && digest == fileDigest(installed)
}

func fileDigest(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	digest := sha256.New()
	if _, err = io.Copy(digest, file); err != nil {
		return ""
	}
	return hex.EncodeToString(digest.Sum(nil))
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
