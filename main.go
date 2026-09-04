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
		JS:                         cornerGestureJS(port),
	})
	appService.mainWindow = mainWindow

	if isKiosk {
		appService.isWebappActive.Store(true)
		menubar.ApplyWebviewZoom(appService.config.GetWebViewZoom())
		mainWindow.Fullscreen()
		mainWindow.HideMenuBar()
		mainWindow.SetMenu(nil)
		menubar.SetNativeMenubarVisible(false)
		menubar.SetNativeFullscreen(true)
		go func() {
			for _, delay := range []time.Duration{100 * time.Millisecond, 300 * time.Millisecond, 800 * time.Millisecond, 1500 * time.Millisecond} {
				time.Sleep(delay)
				if appService.isWebappActive.Load() {
					mainWindow.Fullscreen()
					mainWindow.HideMenuBar()
					mainWindow.SetMenu(nil)
					menubar.SetNativeMenubarVisible(false)
					menubar.SetNativeFullscreen(true)
					menubar.ApplyWebviewZoom(appService.config.GetWebViewZoom())
				}
			}
		}()
	} else if runtime.GOOS != "android" {
		appMenu := createMenu(app, appService)
		app.Menu.Set(appMenu)
	}

	if err := app.Run(); err != nil {
		logger.Errorf("Application error: %v", err)
	}
}

func cornerGestureJS(port int) string {
	return fmt.Sprintf(`(function() {
    if (window.__kioskGestureInitialized) return;
    window.__kioskGestureInitialized = true;
    var lastTap = 0;
    var tapCount = 0;
    var port = %d;
    window.addEventListener('pointerdown', function(e) {
        if (window.location.pathname === '/' && (window.location.hostname === '127.0.0.1' || window.location.hostname === 'localhost')) {
            return;
        }
        var x = e.clientX;
        var y = e.clientY;
        var w = window.innerWidth;
        var h = window.innerHeight;
        var r = 80;
        var isCorner = (x <= r && y <= r) ||
                       (x >= w - r && y <= r) ||
                       (x <= r && y >= h - r) ||
                       (x >= w - r && y >= h - r);
        if (isCorner) {
            var now = Date.now();
            if (now - lastTap > 1200) {
                tapCount = 0;
            }
            lastTap = now;
            tapCount++;
            if (tapCount >= 4) {
                tapCount = 0;
                fetch('http://127.0.0.1:' + port + '/api/webview')
                    .then(function(res) { return res.json(); })
                    .then(function(cfg) {
                        if (cfg && cfg.hasPIN) {
                            var pin = window.prompt("Enter Admin PIN to exit web application:");
                            if (!pin) return;
                            fetch('http://127.0.0.1:' + port + '/api/auth/session', {
                                method: 'POST',
                                headers: { 'Content-Type': 'application/json' },
                                body: JSON.stringify({ pin: pin })
                            }).then(function(authRes) {
                                if (authRes.ok) {
                                    fetch('http://127.0.0.1:' + port + '/api/webview/close', { method: 'POST' });
                                } else {
                                    alert("Incorrect PIN");
                                }
                            });
                        } else {
                            fetch('http://127.0.0.1:' + port + '/api/webview/close', { method: 'POST' });
                        }
                    })
                    .catch(function() {
                        fetch('http://127.0.0.1:' + port + '/api/webview/close', { method: 'POST' });
                    });
            }
        }
    }, true);
})();`, port)
}
