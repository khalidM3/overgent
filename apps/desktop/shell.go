//go:build darwin || linux || windows

package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// runShell is the desktop application, minus the parts each operating system
// answers differently.
//
// Everything here - the window, the deep-link route, the tray menu, and the
// five-second health refresh behind it - is the same product on every platform,
// and was only ever inside main_darwin.go because macOS was the only platform.
// A per-OS main builds a platformShell and hands it over; nothing in this file
// asks which platform it is on.
func runShell(platform platformShell) {
	assets, err := fs.Sub(embeddedAssets, "frontend/embed/app")
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "desktop assets missing; run pnpm desktop:assets")
		os.Exit(1)
	}

	// The router exists before the window does because a second instance can
	// deliver a URL at any moment, including during startup. It holds one until
	// there is a window to show it in.
	router := &deepLinkRouter{}

	options := application.Options{
		Name:        desktopProductName(),
		Description: "Persistent coordination for teams working with coding agents",
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Services: []application.Service{application.NewService(newOnboardingService())},
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
		Mac:      platform.Mac,
		Linux:    platform.Linux,
		Windows:  platform.Windows,
	}
	if platform.SingleInstance != nil {
		options.SingleInstance = platform.SingleInstance(router.deliver)
	}
	app := application.New(options)

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "overgent-dashboard",
		Title:            "Overgent",
		Width:            1320,
		Height:           900,
		MinWidth:         920,
		MinHeight:        680,
		URL:              desktopStartURL(),
		BackgroundColour: application.NewRGB(244, 245, 240),
		Mac:              platform.MacWindow,
		Linux:            platform.LinuxWindow,
		Windows:          platform.WindowsWindow,
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
	router.attach(func(raw string) {
		if projectID, ok := desktopDeepLinkProject(raw); ok {
			go func() {
				if err := openProject(context.Background(), window, desktopConfigRoot(), projectID); err != nil {
					slog.Warn("open deep-linked Project", "error", err)
				}
			}()
			window.Show()
			window.Focus()
			return
		}
		target, ok := desktopDeepLinkTarget(raw)
		if !ok {
			return
		}
		window.SetURL(target)
		window.Show()
		window.Focus()
	})
	// Every platform's Wails backend emits this for a URL the application was
	// launched with. Only macOS emits it again for a link that arrives while the
	// app is already running; the other two reach the same handler through the
	// single-instance callback the per-OS main installs.
	app.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) {
		router.deliver(event.Context().URL())
	})

	tray := app.SystemTray.New()
	platform.ApplyTrayIcon(tray)
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
	// Both buttons should open the one menu: that is what every other extra on
	// the bar does, and a plain click is the only thing a trackpad user thinks
	// to try. A right-click reaches it; on macOS a left-click does not, and on
	// Wails v3.0.0-beta.12 nothing here can make it, because both of the routes
	// the framework offers assume a status item click arrives as an ordinary
	// mouse event. On current macOS it does not. Instrumenting the callbacks
	// showed:
	//
	//   - Wails guards its NSEvent monitor on `event.window != button.window`,
	//     which never matches for these clicks, so the callback that would ask
	//     macOS for native menu tracking is never reached.
	//   - Its action handler reads `[NSApp currentEvent].type`, which comes
	//     back 13/14 (AppKitDefined/SystemDefined) rather than 1. The switch in
	//     processClick tests only leftButtonDown=1 and rightButtonDown=3, so it
	//     falls through and does nothing at all.
	//
	// The handler below is kept because it states the intent and starts working
	// the moment Wails reports the button correctly. Until then the menu is
	// reachable by right-click, and every item in it is also reachable from the
	// window, so nothing is only behind the broken gesture. Revisit on the next
	// Wails bump; if it is still broken, the fix is our own NSStatusItem. It is
	// registered on every platform because the gesture is correct everywhere and
	// only the macOS delivery is broken.
	tray.OnClick(func() { tray.OpenMenu() })

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
		slog.Error("run desktop shell", "error", err)
		os.Exit(1)
	}
}

// platformShell is everything about the shell that an operating system decides.
//
// It is deliberately data rather than an interface: each field is one Wails
// option block or one small hook, so a reader can see the whole per-OS surface
// of the window and tray in one struct instead of tracing method sets.
type platformShell struct {
	// Application-level option blocks. Each is ignored by Wails on the
	// platforms it does not belong to, but they are set only by their own main
	// so that a reader is never asked to guess which one is live.
	Mac     application.MacOptions
	Linux   application.LinuxOptions
	Windows application.WindowsOptions

	// The same, for the one window.
	MacWindow     application.MacWindow
	LinuxWindow   application.LinuxWindow
	WindowsWindow application.WindowsWindow

	// SingleInstance builds the single-instance options, given the shell's own
	// deep-link entry point to forward a second launch's URL to. It is nil on
	// macOS, where the OS re-activates the running application and Wails emits
	// the launch-with-URL event again, so a second process never starts.
	SingleInstance func(deliver func(string)) *application.SingleInstanceOptions

	// ApplyTrayIcon installs the status icon. macOS wants a template image it
	// tints itself; the other two want real pixels, and a dark-mode variant.
	ApplyTrayIcon func(*application.SystemTray)
}

// deepLinkRouter delivers a scheme URL to the window once there is one.
//
// The window is built after the application, and on Linux and Windows a second
// instance can hand over a URL before that has happened. Dropping it would make
// "open Overgent from a link while it is already running" work only some of the
// time, which is worse than not working at all. Only the most recent URL is
// held: they are all navigation requests, and a queue of stale ones is not
// something a member asked for.
type deepLinkRouter struct {
	mu      sync.Mutex
	handle  func(string)
	pending string
}

func (router *deepLinkRouter) attach(handle func(string)) {
	router.mu.Lock()
	router.handle = handle
	pending := router.pending
	router.pending = ""
	router.mu.Unlock()
	if pending != "" {
		handle(pending)
	}
}

func (router *deepLinkRouter) deliver(raw string) {
	if raw == "" {
		return
	}
	router.mu.Lock()
	handle := router.handle
	if handle == nil {
		router.pending = raw
	}
	router.mu.Unlock()
	if handle != nil {
		handle(raw)
	}
}
