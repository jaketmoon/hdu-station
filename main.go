package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app, err := createApplication()
	if err != nil {
		log.Print(err)
		return
	}

	frontendAssets, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Print(err)
		return
	}
	_, background, _ := startupPalette(app.savedTheme())
	err = wails.Run(&options.App{
		Title:     "HDU Station",
		Width:     1180,
		Height:    780,
		MinWidth:  380,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: themeAssets{FS: frontendAssets, theme: app.savedTheme},
		},
		BackgroundColour: &background,
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Printf("HDU Station stopped: %v", err)
	}
}
