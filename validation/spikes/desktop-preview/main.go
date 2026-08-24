package main

import (
	"embed"
	"log"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := application.New(application.Options{
		Name:        "Stickguy Desktop Spike",
		Description: "Disposable native window and menu-bar validation",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyRegular,
		},
	})

	tray := app.SystemTray.New()
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "stickguy-preview",
		Title:            "Stickguy — Desktop Preview",
		Width:            1180,
		Height:           780,
		MinWidth:         860,
		MinHeight:        620,
		URL:              "/",
		BackgroundColour: application.NewRGB(244, 241, 234),
		KeyBindings: map[string]func(application.Window){
			"F12": func(application.Window) { tray.OpenMenu() },
		},
		Mac: application.MacWindow{
			TitleBar:    application.MacTitleBarHiddenInset,
			TabbingMode: application.MacWindowTabbingModeDisallowed,
		},
	})

	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		window.Hide()
		event.Cancel()
	})

	tray.SetTooltip("Stickguy · service disconnected")
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(icons.SystrayMacTemplate)
	}

	menu := app.NewMenu()
	menu.Add("Stickguy preview").SetEnabled(false)
	statusItem := menu.Add("Service: disconnected").SetEnabled(false)
	menu.Add("Projects: fixture mode").SetEnabled(false)
	menu.AddSeparator()
	menu.Add("Open Stickguy").OnClick(func(*application.Context) {
		window.Show()
		window.Focus()
	})
	paused := false
	pauseItem := menu.Add("Pause sharing")
	pauseItem.OnClick(func(*application.Context) {
		paused = !paused
		if paused {
			pauseItem.SetLabel("Resume sharing")
			statusItem.SetLabel("Service: preview paused")
			tray.SetTooltip("Stickguy · preview paused")
			return
		}
		pauseItem.SetLabel("Pause sharing")
		statusItem.SetLabel("Service: disconnected")
		tray.SetTooltip("Stickguy · service disconnected")
	})
	menu.Add("Scan now").SetEnabled(false).SetTooltip("Available after the local service is connected")
	menu.AddSeparator()
	menu.Add("Quit Stickguy Preview").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	tray.OnClick(func() {
		window.Show()
		window.Focus()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
