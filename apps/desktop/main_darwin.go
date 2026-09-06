//go:build darwin

package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

func main() {
	assets, err := fs.Sub(embeddedAssets, "frontend/embed/app")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "desktop assets missing; run pnpm desktop:assets")
		os.Exit(1)
	}

	app := application.New(application.Options{
		Name:        desktopProductName(),
		Description: "Persistent coordination for teams working with coding agents",
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Services: []application.Service{application.NewService(newOnboardingService())},
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyRegular,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "overgent-dashboard",
		Title:            "Overgent",
		Width:            1320,
		Height:           900,
		MinWidth:         920,
		MinHeight:        680,
		URL:              desktopStartURL(),
		BackgroundColour: application.NewRGB(244, 245, 240),
		Mac: application.MacWindow{
			// One bar, and it is the app's own. A separate native title bar spent
			// a strip of every window on a word the sidebar already says, and put
			// a system-drawn edge between the window frame and an interface whose
			// whole look is hairlines. Hidden-inset keeps the traffic lights where
			// macOS puts them and hands the rest of that strip to the page; the
			// sidebar reserves the top-left corner for them and marks the strip
			// draggable (`--wails-draggable`, style.css "desktop shell"), so the
			// window still moves by its top edge.
			TitleBar:    application.MacTitleBarHiddenInset,
			TabbingMode: application.MacWindowTabbingModeDisallowed,
			WebviewPreferences: application.MacWebviewPreferences{
				// This window navigates to the hosted origin to show a live
				// Project, and a hosted page has no native bridge to ask where
				// it is running. The name is appended to the webview's user
				// agent, so that page can still tell it is inside the app and
				// say "continue on this Mac" instead of offering to open the
				// app the member is already looking at. It grants nothing: the
				// bridge stays unreachable from any origin but this one.
				ApplicationNameForUserAgent: desktopUserAgentName,
			},
		},
	})
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		window.Hide()
		event.Cancel()
	})

	// The live Project view is served from the hosted origin, where the native
	// bridge does not exist, so it cannot register a repository with the local
	// service. Rather than telling a member to open a terminal, it hands control
	// back here through the registered scheme and this brings the app forward on
	// the add-Project screen.
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) {
		if projectID, ok := desktopDeepLinkProject(event.Context().URL()); ok {
			go func() {
				if err := openProject(context.Background(), window, desktopConfigRoot(), projectID); err != nil {
					slog.Warn("open deep-linked Project", "error", err)
				}
			}()
			window.Show()
			window.Focus()
			return
		}
		target, ok := desktopDeepLinkTarget(event.Context().URL())
		if !ok {
			return
		}
		window.SetURL(target)
		window.Show()
		window.Focus()
	})

	tray := app.SystemTray.New()
	tray.SetTemplateIcon(menuBarIcon())
	menu := app.NewMenu()
	menu.Add(desktopMenuLabel()).SetEnabled(false)
	serviceItem := menu.Add("Service: checking…").SetEnabled(false)
	activityItem := menu.Add("Activity: checking…").SetEnabled(false)
	// Only a local-mode profile has a backend of its own, so this line appears
	// only where it means something.
	backendItem := menu.Add("").SetHidden(true).SetEnabled(false)
	menu.AddSeparator()
	menu.Add("Open Overgent").OnClick(func(*application.Context) {
		window.Show()
		window.Focus()
	})
	menu.Add("Open live Project").OnClick(func(*application.Context) {
		go func() {
			if err := openLocalProject(context.Background(), window); err != nil {
				slog.Warn("open local Project", "error", err)
			}
		}()
	})
	// Once the window is showing a live Project it is on the hosted origin, and
	// nothing on that page can reach the local service. This item is the one
	// route back to the screen that can register a repository which does not
	// depend on the webview handing a custom scheme to the system.
	menu.Add("Add a project…").OnClick(func(*application.Context) {
		window.SetURL(addProjectURL)
		window.Show()
		window.Focus()
	})
	pauseItem := menu.Add("Pause sharing everywhere").SetEnabled(false)
	// A mute nobody remembers setting is the failure mode of muting, so this
	// line exists only while something is actually muted and its whole job is
	// to be noticed on the day it matters.
	focusItem := menu.Add("").SetHidden(true)
	scanItem := menu.Add("Scan now").SetEnabled(false)
	menu.AddSeparator()
	menu.Add("Quit Overgent").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	tray.OnClick(func() {
		window.Show()
		window.Focus()
	})

	control := controller{service: newDaemonService()}
	var stateMu sync.RWMutex
	current := ServiceStatus{}
	refresh := func() {
		status := control.status(context.Background())
		stateMu.Lock()
		current = status
		stateMu.Unlock()
		serviceItem.SetLabel(status.ServiceLabel())
		activityItem.SetLabel(status.ActivityLabel())
		if label := status.BackendLabel(); label == "" {
			backendItem.SetHidden(true)
		} else {
			backendItem.SetLabel(label).SetHidden(false)
		}
		pauseItem.SetLabel(status.PauseLabel()).SetEnabled(status.Connected && status.WorkspaceCount > 0)
		if label := status.FocusLabel(); label == "" {
			focusItem.SetHidden(true)
		} else {
			focusItem.SetLabel(label).SetHidden(false).SetEnabled(true)
		}
		scanItem.SetEnabled(status.Connected)
		tray.SetTooltip(status.Tooltip())
	}
	pauseItem.OnClick(func(*application.Context) {
		stateMu.RLock()
		status := current
		stateMu.RUnlock()
		go func() {
			if err := control.togglePause(context.Background(), status); err != nil {
				return
			}
			refresh()
		}()
	})
	focusItem.OnClick(func(*application.Context) {
		go func() {
			if err := control.clearFocus(context.Background()); err != nil {
				slog.Warn("clear session focus", "error", err)
				return
			}
			refresh()
		}()
	})
	scanItem.OnClick(func(*application.Context) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			if err := control.service.Scan(ctx); err == nil {
				refresh()
			}
		}()
	})
	go func() {
		refresh()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-app.Context().Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()

	if err := app.Run(); err != nil {
		slog.Error("run desktop preview", "error", err)
		os.Exit(1)
	}
}

func menuBarIcon() []byte {
	canvas := image.NewRGBA(image.Rect(0, 0, 18, 18))
	ink := color.RGBA{A: 255}
	draw.Draw(canvas, image.Rect(7, 1, 11, 5), &image.Uniform{C: ink}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(8, 5, 10, 12), &image.Uniform{C: ink}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(4, 7, 14, 9), &image.Uniform{C: ink}, image.Point{}, draw.Src)
	for offset := range 4 {
		canvas.Set(7-offset, 12+offset, ink)
		canvas.Set(10+offset, 12+offset, ink)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		return nil
	}
	return encoded.Bytes()
}
