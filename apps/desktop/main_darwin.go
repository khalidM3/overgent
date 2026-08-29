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
		Name:             "stickguy-dashboard",
		Title:            "Stickguy",
		Width:            1320,
		Height:           900,
		MinWidth:         920,
		MinHeight:        680,
		URL:              desktopStartURL(),
		BackgroundColour: application.NewRGB(244, 245, 240),
		Mac: application.MacWindow{
			// Keep the native title bar outside the web content. Full-size content
			// places the traffic lights over the Project sidebar and brand mark.
			TitleBar:    application.MacTitleBarDefault,
			TabbingMode: application.MacWindowTabbingModeDisallowed,
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
	menu.AddSeparator()
	menu.Add("Open Stickguy").OnClick(func(*application.Context) {
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
	pauseItem := menu.Add("Pause all sharing").SetEnabled(false)
	scanItem := menu.Add("Scan now").SetEnabled(false)
	menu.AddSeparator()
	menu.Add("Quit Stickguy").OnClick(func(*application.Context) { app.Quit() })
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
		pauseItem.SetLabel(status.PauseLabel()).SetEnabled(status.Connected && status.WorkspaceCount > 0)
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
