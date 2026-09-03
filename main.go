package main

import (
	"embed"
	"os"
	"runtime"

	"epos-proxy/internal/logger"
	"epos-proxy/override/menubar"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if runtime.GOOS == "linux" {
		if os.Getenv("WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS") == "" {
			_ = os.Setenv("WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS", "1")
		}
		if os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") == "" {
			_ = os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
		}
		for _, p := range []string{
			"/usr/lib/x86_64-linux-gnu/gio/modules",
			"/usr/lib/aarch64-linux-gnu/gio/modules",
			"/usr/lib64/gio/modules",
			"/usr/lib/gio/modules",
		} {
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				if os.Getenv("GIO_EXTRA_MODULES") == "" {
					_ = os.Setenv("GIO_EXTRA_MODULES", p)
				}
				break
			}
		}
	}

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

	startURL := "/"
	isKiosk := appService.config.GetWebViewEnabled() && appService.config.GetWebViewURL() != ""
	if isKiosk {
		startURL = appService.config.GetWebViewURL()
		if startState != application.WindowStateMinimised {
			startState = application.WindowStateFullscreen
		}
	}

	isDev := appService.config.IsDevMode()

	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:                      "ePOS Proxy",
		Width:                      800,
		Height:                     600,
		MinWidth:                   700,
		MinHeight:                  500,
		StartState:                 startState,
		BackgroundColour:           application.NewRGB(255, 255, 255),
		URL:                        startURL,
		UseApplicationMenu:         true,
		DevToolsEnabled:            isDev,
		DefaultContextMenuDisabled: !isDev,
	})
	appService.mainWindow = mainWindow

	if isKiosk {
		mainWindow.HideMenuBar()
		menubar.SetNativeMenubarVisible(false)
	}

	if runtime.GOOS != "android" {
		appMenu := createMenu(app, appService)
		app.Menu.Set(appMenu)
	}

	appService.startup()

	if err := app.Run(); err != nil {
		logger.Errorf("Application error: %v", err)
	}
}
