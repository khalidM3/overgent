//go:build linux

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// registerDeepLinkScheme claims overgent:// for this build, per user.
//
// There is no manifest on Linux: a scheme handler is a .desktop file carrying
// MimeType=x-scheme-handler/<scheme>, and the desktop environment finds it by
// reading an index that update-desktop-database maintains. Both are written
// here, on every launch, rather than by a packaging step, for two reasons:
//
//   - An AppImage or a tarball has no install step at all, and those are the
//     two ways most people will first run this.
//   - Exec= has to name where the executable actually is. A package that wrote
//     the file once would be stale the moment the member moved the application,
//     and the link would launch nothing with no visible error.
//
// It is per user (~/.local/share) and never system-wide: claiming a scheme for
// every account on a shared machine is not something a member starting an app
// has asked for.
func registerDeepLinkScheme() error {
	executable, err := launchCommandPath()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if !filepath.IsAbs(dataHome) {
		dataHome = filepath.Join(home, ".local", "share")
	}
	applications := filepath.Join(dataHome, "applications")
	if err = os.MkdirAll(applications, 0o700); err != nil {
		return fmt.Errorf("create the desktop entry directory: %w", err)
	}
	// The entry names a themed icon either way. Installing the file is what
	// makes the theme able to find one, and failing to is a cosmetic fault:
	// refusing to register the scheme over it would trade a generic icon for a
	// dead link.
	if err = installDesktopIcon(dataHome); err != nil {
		slog.Debug("install the application icon", "error", err)
	}

	entry := desktopEntry(executable)
	path := filepath.Join(applications, desktopEntryName()+".desktop")
	if err = writeFileAtomically(path, []byte(entry), 0o644); err != nil {
		return fmt.Errorf("write the desktop entry: %w", err)
	}

	// update-desktop-database rebuilds the mimeinfo cache the desktop reads to
	// answer "who handles this scheme". Without it the file is present and
	// ignored until something else happens to trigger a rebuild. It is absent on
	// some minimal systems, where the desktop reads the directory directly, so
	// its absence is not an error.
	tool, lookErr := exec.LookPath("update-desktop-database")
	if lookErr != nil {
		return nil
	}
	command := exec.Command(tool, applications)
	if err = command.Start(); err != nil {
		return nil
	}
	// Bound it rather than waiting: this runs on the path to showing a window,
	// and a slow cache rebuild must not hold the application closed.
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
	}
	return nil
}

// launchCommandPath is the path a desktop file should launch.
//
// Inside an AppImage os.Executable() points into a FUSE mount that only exists
// while the application is running, so a .desktop file naming it would work
// once and then launch nothing. The runtime exports APPIMAGE with the path of
// the .AppImage file itself, which is the thing that is still there tomorrow.
func launchCommandPath() (string, error) {
	if image := strings.TrimSpace(os.Getenv("APPIMAGE")); filepath.IsAbs(image) {
		if info, err := os.Stat(image); err == nil && info.Mode().IsRegular() {
			return image, nil
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(executable)
}

// installDesktopIcon puts the application icon where the icon theme looks for
// it, under the name the desktop entry uses.
//
// The hicolor theme is the fallback every icon theme inherits from, so a PNG
// placed there is found whichever theme the member uses. The entry names the
// icon by that bare name rather than by an absolute path, which is what lets
// the desktop pick the size it wants instead of scaling one.
func installDesktopIcon(dataHome string) error {
	resources, err := bundledResourceDirectory()
	if err != nil {
		return err
	}
	body, err := os.ReadFile(filepath.Join(resources, "icons", desktopEntryName()+".png"))
	if err != nil {
		return err
	}
	directory := filepath.Join(dataHome, "icons", "hicolor", "512x512", "apps")
	if err = os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	return writeFileAtomically(filepath.Join(directory, desktopEntryName()+".png"), body, 0o644)
}
