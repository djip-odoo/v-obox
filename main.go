package main

import (
	"embed"
	"fmt"
	"os"
	"runtime"
	"time"

	"epos-proxy/internal/logger"
	"epos-proxy/override/menubar"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

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
	appService.startup()

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
	port := 4545
	if appService.webserver != nil && appService.webserver.Port > 0 {
		port = appService.webserver.Port
	}

	initJS := cornerGestureJS(port, appService.config.HasWebViewPIN())
	zoom := appService.config.GetWebViewZoom()
	if zoom <= 0 {
		zoom = 1.0
	}
	initJS += "\n" + buildZoomJS(zoom, port, appService.config.GetWebViewURL())

	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:                      "ePOS Proxy",
		Width:                      800,
		Height:                     600,
		MinWidth:                   700,
		MinHeight:                  500,
		StartState:                 startState,
		BackgroundColour:           application.NewRGB(255, 255, 255),
		URL:                        startURL,
		UseApplicationMenu:         !isKiosk,
		DevToolsEnabled:            isDev,
		DefaultContextMenuDisabled: !isDev,
		JS:                         initJS,
	})
	appService.mainWindow = mainWindow

	var isQuitting bool
	mainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if isQuitting {
			return
		}
		if appService.ConfirmQuit() {
			isQuitting = true
			logger.Infof("Quit requested by user")
			app.Quit()
		} else {
			e.Cancel()
		}
	})

	if isKiosk {
		appService.isWebappActive.Store(true)
		menubar.ApplyWebviewZoom(appService.config.GetWebViewZoom())
		menubar.SetNativeMenubarVisible(false)
		menubar.SetNativeFullscreen(true)
		go func() {
			for _, delay := range []time.Duration{100 * time.Millisecond, 300 * time.Millisecond, 800 * time.Millisecond, 1500 * time.Millisecond} {
				time.Sleep(delay)
				if appService.isWebappActive.Load() {
					menubar.SetNativeMenubarVisible(false)
					menubar.SetNativeFullscreen(true)
					menubar.ApplyWebviewZoom(appService.config.GetWebViewZoom())
				}
			}
		}()
	}

	if runtime.GOOS != "android" {
		if !isKiosk {
			appMenu := createMenu(app, appService)
			app.Menu.Set(appMenu)
		}

		systemTray := app.SystemTray.New()
		if len(appIcon) > 0 {
			systemTray.SetIcon(appIcon)
		}
		if runtime.GOOS == "darwin" {
			systemTray.SetTemplateIcon(icons.SystrayMacTemplate)
		}
		systemTray.SetTooltip("ePOS Proxy")
		systemTray.SetMenu(createTrayMenu(app, appService))
		systemTray.OnClick(func() {
			showMainWindow(appService)
		})
	}

	if err := app.Run(); err != nil {
		logger.Errorf("Application error: %v", err)
	}
}

func cornerGestureJS(port int, hasPIN bool) string {
	return fmt.Sprintf(`(function() {
    if (window.__kioskGestureInitialized) return;
    window.__kioskGestureInitialized = true;
    var lastTap = 0;
    var tapCount = 0;
    var port = %d;
    var requiresPIN = %t;
    function handleTap(e) {
        if (window.location.pathname === '/' && (window.location.hostname === '127.0.0.1' || window.location.hostname === 'localhost')) {
            return;
        }
        var x = e.clientX;
        var y = e.clientY;
        if (x === undefined && e.touches && e.touches.length > 0) {
            x = e.touches[0].clientX;
            y = e.touches[0].clientY;
        }
        if (x === undefined && e.changedTouches && e.changedTouches.length > 0) {
            x = e.changedTouches[0].clientX;
            y = e.changedTouches[0].clientY;
        }
        if (x === undefined || y === undefined) return;
        var w = window.innerWidth;
        var h = window.innerHeight;
        var r = 120;
        var isCorner = (x <= r && y >= h - r) ||
                       (x >= w - r && y >= h - r);
        if (isCorner) {
            var now = Date.now();
            if (now - lastTap < 80) return;
            if (now - lastTap > 2000) {
                tapCount = 0;
            }
            lastTap = now;
            tapCount++;
            if (tapCount >= 4) {
                tapCount = 0;
                var target = 'http://127.0.0.1:' + port + '/api/webview/close';
                if (requiresPIN) {
                    var pin = window.prompt("Enter Admin PIN to return to Wails app:");
                    if (!pin) return;
                    target += '?pin=' + encodeURIComponent(pin);
                }
                window.location.href = target;
            }
        }
    }
    window.addEventListener('pointerdown', handleTap, true);
    window.addEventListener('touchstart', handleTap, true);
    window.addEventListener('mousedown', handleTap, true);
})();`, port, hasPIN)
}
