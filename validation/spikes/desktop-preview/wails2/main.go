package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := wails.Run(&options.App{
		Title:     "Stickguy — Wails v2 fallback",
		Width:     1180,
		Height:    780,
		MinWidth:  860,
		MinHeight: 620,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 244, G: 241, B: 234, A: 255},
	}); err != nil {
		log.Fatal(err)
	}
}
