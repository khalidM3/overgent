//go:build darwin && production

package main

import (
	"context"
	"errors"

	"github.com/stickguy/stickguy/internal/config"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const desktopDevelopment = false

func desktopProductName() string       { return "Stickguy" }
func desktopMenuLabel() string         { return "Stickguy desktop preview" }
func desktopStartURL() string          { return "/?desktop=preview" }
func desktopAPIBaseURL() string        { return "https://api.stickguy.dev" }
func desktopActivationBaseURL() string { return desktopAPIBaseURL() }
func desktopCLIBinary() string         { return "" }
func desktopConfigRoot() string        { root, _ := config.DefaultRoot(); return root }
func openLocalProject(context.Context, *application.WebviewWindow) error {
	return errors.New("local Project activation is available only in the development app")
}
