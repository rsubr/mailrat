package main

import (
	"embed"
	"io/fs"

	"github.com/mailrat/mailrat/internal/desktop"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend
var rawAssets embed.FS

func main() {
	frontendFS, err := fs.Sub(rawAssets, "frontend")
	if err != nil {
		panic(err)
	}

	app := desktop.NewApp()
	if err := wails.Run(&options.App{
		Title:     "mailrat",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: frontendFS,
		},
		OnStartup:  app.OnStartup,
		OnShutdown: app.OnShutdown,
		Bind:       []interface{}{app},
	}); err != nil {
		println("Error:", err.Error())
	}
}
