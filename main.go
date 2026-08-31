package main

import (
	"embed"
	"os"
	"runtime"

	"epos-proxy/internal/logger"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	logger.InitLogger()
	logger.Debugf("Starting ePOS Proxy")

	appService := NewApp()

	app := application.New(application.Options{
		Name:        "ePOS Proxy",
		Description: "Expose USB and network printers as HTTP endpoints",
		Services: []application.Service{
			application.NewService(appService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	appService.wailsApp = app

	startState := application.WindowStateNormal
	for _, arg := range os.Args[1:] {
		if arg == "--minimized" {
			logger.Debugf("Application started with --minimized flag")
			startState = application.WindowStateMinimised
			break
		}
	}

	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:              "ePOS Proxy",
		Width:              800,
		Height:             600,
		MinWidth:           700,
		MinHeight:          500,
		StartState:         startState,
		BackgroundColour:   application.NewRGB(255, 255, 255),
		URL:                "/",
		UseApplicationMenu: true,
	})
	appService.mainWindow = mainWindow

	if runtime.GOOS != "android" {
		appMenu := createMenu(app, appService)
		app.Menu.Set(appMenu)
	}

	appService.startup()

	if err := app.Run(); err != nil {
		logger.Errorf("Application error: %v", err)
	}
}
