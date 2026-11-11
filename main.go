package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend
var embeddedAssets embed.FS

func main() {
	assets, err := fs.Sub(embeddedAssets, "frontend")
	if err != nil {
		log.Fatal(err)
	}
	windowsOptions := &windows.Options{}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		windowsOptions.WebviewUserDataPath = filepath.Join(localAppData, "Iperf3GUI", "webview")
	}

	app := NewApp()
	err = wails.Run(&options.App{
		Title:            "iPerf3 网络测试台",
		Width:            1280,
		Height:           820,
		MinWidth:         1080,
		MinHeight:        680,
		BackgroundColour: &options.RGBA{R: 243, G: 245, B: 247, A: 1},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Windows:    windowsOptions,
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
