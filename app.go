package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"
	"epos-proxy/internal/server"
	"epos-proxy/internal/util"
	"epos-proxy/override/menubar"

	autostart "github.com/emersion/go-autostart"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// dialoger abstracts native GUI dialogs for testability.
type dialoger interface {
	Error(title, message string)
	Question(title, message string) bool
	SaveFile(opts *application.SaveFileDialogOptions) (string, error)
}

type defaultDialogs struct{}

func (defaultDialogs) Error(title, message string) {
	if app := application.Get(); app != nil {
		app.Dialog.Error().SetTitle(title).SetMessage(message).Show()
	} else {
		logger.Errorf("%s: %s", title, message)
	}
}

func (defaultDialogs) Question(title, message string) bool {
	confirmed := false
	if app := application.Get(); app != nil {
		dialog := app.Dialog.Question().SetTitle(title).SetMessage(message)
		btnCancel := dialog.AddButton("Cancel")
		btnCancel.SetAsCancel().SetAsDefault()
		btnConfirm := dialog.AddButton("Confirm")
		btnConfirm.OnClick(func() {
			confirmed = true
		})
		dialog.Show()
	}
	return confirmed
}

func (defaultDialogs) SaveFile(opts *application.SaveFileDialogOptions) (string, error) {
	if app := application.Get(); app != nil {
		return app.Dialog.SaveFileWithOptions(opts).PromptForSingleSelection()
	}
	return "", nil
}

// App struct
type App struct {
	wailsApp       *application.App
	mainWindow     *application.WebviewWindow
	webserver      *server.Server
	config         *config.Manager
	printerManager *printer.Manager
	autoStart      *autostart.App
	dialogs        dialoger
	sessionToken   string // trusted Wails-origin token set once in startup()
	isWebappActive atomic.Bool
}

func (a *App) dlg() dialoger {
	if a.dialogs == nil {
		return defaultDialogs{}
	}
	return a.dialogs
}

// EmitEvent broadcasts a custom event to the frontend.
func (a *App) EmitEvent(name string, data ...any) {
	if a.wailsApp != nil {
		a.wailsApp.Event.Emit(name, data...)
	} else if app := application.Get(); app != nil {
		app.Event.Emit(name, data...)
	}
}

// showError surfaces an error to the user via a native dialog.
func (a *App) showError(title, message string) {
	a.dlg().Error(title, message)
}

type Printer struct {
	Name   string `json:"name"`
	Ip     string `json:"ip"`
	Id     string `json:"id"`
	IsLAN  bool   `json:"isLAN"`
	LANIp  string `json:"lanIp,omitempty"`
	Online bool   `json:"online"`
	Type   string `json:"type"`
}

type UnavailablePrinter struct {
	Name     string `json:"name"`
	ErrorMsg string `json:"errorMsg"`
	IsLAN    bool   `json:"isLAN"`
	LANIp    string `json:"lanIp,omitempty"`
}

type AppVariable struct {
	ServerRunning bool   `json:"serverRunning"`
	Os            string `json:"os"`
	Mode          string `json:"mode"`
}

// WebViewConfig is the public view of kiosk settings (PIN is never exposed).
type WebViewConfig struct {
	URL      string  `json:"url"`
	Enabled  bool    `json:"enabled"`
	HasPIN   bool    `json:"hasPIN"`
	Zoom     float64 `json:"zoom"`
	IsActive bool    `json:"isActive"`
}

type Printers struct {
	ErrorMsg            string               `json:"errorMsg"`
	Printers            []Printer            `json:"printers"`
	UnavailablePrinters []UnavailablePrinter `json:"unavailablePrinters"`
}

func NewApp() *App {
	a := &App{}

	a.autoStart = &autostart.App{
		Name:        "epos-proxy",
		DisplayName: "ePOS Proxy",
		Exec:        []string{os.Args[0]},
	}
	a.printerManager = printer.NewManager()

	cfg, err := config.NewManager()
	if err != nil {
		logger.Fatalf("Config initialization failed: %v", err)
	}

	if err := cfg.Load(); err != nil {
		logger.Warnf("Config load warning: %v", err)
	}

	a.config = cfg

	return a
}

func (a *App) startup() {
	logger.Debugf("Application startup")
	logger.Debugf("Config loaded from %s", a.config.Path())

	port, err := a.config.ResolvePort()
	if err != nil {
		logger.Warn("Unable to resolve port, using default")
	}

	// Build a sub-FS rooted at frontend/dist for the embedded SPA.
	var distFS fs.FS
	subFS, fsErr := fs.Sub(assets, "frontend/dist")
	if fsErr != nil {
		logger.Warnf("Could not create distFS sub: %v", fsErr)
	} else {
		distFS = subFS
	}

	a.webserver = server.New(port, a.printerManager, a.config, distFS)

	// Generate a unique session token that identifies requests from this
	// trusted Wails process. The remote webview never has this token.
	token := uuid.New().String()
	a.sessionToken = token
	a.webserver.SetSessionToken(token)

	// Notify the desktop frontend when kiosk status or config is modified remotely
	a.webserver.SetKioskCallback(func(enabled bool) {
		a.EmitEvent("kiosk-state-changed", enabled)
		if a.isWebappActive.Load() {
			a.SetWindowFullscreen(enabled)
		}
		a.EmitEvent("webview-config-changed")
	})
	a.webserver.SetConfigCallback(func() {
		// Re-apply zoom to the desktop webview when config is changed remotely
		if a.isWebappActive.Load() && a.mainWindow != nil {
			zoom := a.config.GetWebViewZoom()
			if zoom > 0 {
				menubar.ApplyWebviewZoom(zoom)
				port := 4545
				targetUrl := ""
				if a.config != nil {
					targetUrl = a.config.GetWebViewURL()
				}
				a.mainWindow.ExecJS(buildZoomJS(zoom, port, targetUrl))
			}
		}
		a.EmitEvent("webview-config-changed")
	})
	a.webserver.SetKioskReloadCallback(func() {
		a.ReloadKiosk()
	})
	menubar.RegisterKioskReloadCallback(func() {
		logger.Infof("Native WebKitGTK kiosk reload requested from webapp")
		a.ReloadKiosk()
	})
	menubar.RegisterWebviewPageLoadCallback(func() {
		if a.isWebappActive.Load() && a.mainWindow != nil {
			zoom := a.config.GetWebViewZoom()
			port := 4545
			if a.webserver != nil && a.webserver.Port > 0 {
				port = a.webserver.Port
			}
			targetUrl := ""
			if a.config != nil {
				targetUrl = a.config.GetWebViewURL()
			}
			a.mainWindow.ExecJS(cornerGestureJS(port, a.config.HasWebViewPIN()))
			a.mainWindow.ExecJS(buildZoomJS(zoom, port, targetUrl))
		}
	})
	a.webserver.SetOpenWebappCallback(func(url string) {
		a.NavigateToWebapp(url)
	})
	a.webserver.SetCloseWebappCallback(func() {
		a.NavigateToLocalUI()
	})

	menubar.RegisterKioskExitGesture(func() {
		logger.Infof("Native kiosk exit gesture triggered")
		if !a.isWebappActive.Load() {
			return
		}
		if a.config != nil && a.config.HasWebViewPIN() {
			port := 4545
			if a.webserver != nil && a.webserver.Port > 0 {
				port = a.webserver.Port
			}
			promptJS := fmt.Sprintf(`(function() {
				var pin = window.prompt("Enter Admin PIN to return to Wails app:");
				if (pin) {
					window.location.href = "http://127.0.0.1:%d/api/webview/close?pin=" + encodeURIComponent(pin);
				}
			})();`, port)
			if a.mainWindow != nil {
				a.mainWindow.ExecJS(promptJS)
			}
		} else {
			a.NavigateToLocalUI()
		}
	})

	menubar.ConfigureWebviewSettings()
}

func (a *App) shutdown(_ context.Context) {
	logger.Infof("Stopping proxy server")

	if a.webserver != nil {
		if err := a.webserver.Stop(); err != nil {
			logger.Errorf("Server stop error: %v", err)
		}
	}
}

func (a *App) AppVariable() AppVariable {
	running := false
	if a.webserver != nil {
		running = a.webserver.Running()
	}
	mode := "prod"
	if a.config != nil {
		mode = a.config.GetMode()
	}
	return AppVariable{
		Os:            runtime.GOOS,
		ServerRunning: running,
		Mode:          mode,
	}
}

// GetSessionToken returns the per-launch session token that identifies HTTP
// requests from this trusted Wails process. Called once by the frontend on
// startup; the token is never embedded in the built JS bundle.
func (a *App) GetSessionToken() string {
	return a.sessionToken
}

func (a *App) GetPrinterUrl(id string) string {
	port := 8069
	if a.webserver != nil {
		port = a.webserver.Port
	}
	url := fmt.Sprintf("%s:%d/p/%s", util.GetLocalIP(a.config.IsNetworkPrintingEnabled()), port, id)
	logger.Debugf("Generated printer endpoint: %s", url)
	return url
}

func (a *App) Printers() Printers {
	logger.Debug("Collecting printer status")

	printers := make([]Printer, 0)
	unavailablePrinters := make([]UnavailablePrinter, 0)

	printerInfos, err := printer.ListUSBPrinters()
	errorMsg := ""
	if err == nil {
		logger.Debugf("Detected %d available USB printers", len(printerInfos.Available))

		for _, info := range printerInfos.Available {
			printers = append(printers, Printer{
				Id:     info.Id,
				Name:   info.Name,
				Ip:     a.GetPrinterUrl(info.Id),
				Online: true,
				Type:   string(info.Type),
			})
		}

		for _, info := range printerInfos.Unavailable {
			unavailablePrinters = append(unavailablePrinters, UnavailablePrinter{
				Name:     info.Name,
				ErrorMsg: info.Error,
			})

			logger.Warnf("USB printer unavailable: %s (%s)", info.Name, info.Error)
		}
	} else {
		errorMsg = err.Error()
		logger.Errorf("USB printer detection failed: %v", err)
	}

	lanPrinters := printer.ListLANPrinters(a.config)

	for _, info := range lanPrinters {
		printers = append(printers, Printer{
			Id:    info.Id,
			Name:  fmt.Sprintf("Network - %s", info.IP),
			Ip:    a.GetPrinterUrl(info.Id),
			IsLAN: true,
			LANIp: info.IP,
			Type:  string(printer.TypeReceipt),
		})
	}

	return Printers{
		Printers:            printers,
		UnavailablePrinters: unavailablePrinters,
		ErrorMsg:            errorMsg,
	}
}

func (a *App) AddLANPrinter(ip string) error {
	logger.Debugf("Adding LAN printer: %s", ip)

	ip, err := printer.ValidateIPAddress(ip)
	if err != nil {
		return fmt.Errorf("invalid IP address: %s, error: %v", ip, err)
	}

	if err := printer.CheckLANPrinter(ip); err != nil {
		return fmt.Errorf("LAN printer unreachable: %s, error: %v", ip, err)
	}

	if err := a.config.AddLanEposPrinter(ip); err != nil {
		return fmt.Errorf("failed to save LAN printer: %s, error: %v", ip, err)
	}

	logger.Debugf("LAN printer added successfully: %s", ip)
	return nil
}

// ─── WebView / Kiosk ──────────────────────────────────────────────────────────

// GetWebViewConfig returns the public kiosk configuration (URL, enabled flag,
// and whether a PIN has been set). The PIN itself is never returned.
func (a *App) GetWebViewConfig() WebViewConfig {
	return WebViewConfig{
		URL:      a.config.GetWebViewURL(),
		Enabled:  a.config.GetWebViewEnabled(),
		HasPIN:   a.config.HasWebViewPIN(),
		Zoom:     a.config.GetWebViewZoom(),
		IsActive: a.isWebappActive.Load(),
	}
}

// SetWebViewURL persists the kiosk URL.
func (a *App) SetWebViewURL(url string) error {
	logger.Debugf("Setting WebView URL")
	return a.config.SetWebViewURL(url)
}

// SetWebViewZoom persists the kiosk display zoom level. If the Web App is currently active, it applies zoom immediately.
func (a *App) SetWebViewZoom(zoom float64) error {
	logger.Debugf("Setting WebView zoom: %v", zoom)
	if err := a.config.SetWebViewZoom(zoom); err != nil {
		return err
	}
	if a.isWebappActive.Load() {
		menubar.ApplyWebviewZoom(zoom)
		if a.mainWindow != nil {
			port := 4545
			if a.webserver != nil && a.webserver.Port > 0 {
				port = a.webserver.Port
			}
			targetUrl := ""
			if a.config != nil {
				targetUrl = a.config.GetWebViewURL()
			}
			a.mainWindow.ExecJS(buildZoomJS(zoom, port, targetUrl))
		}
	}
	if a.webserver != nil {
		a.webserver.PostWebviewAction("zoom", "", false, zoom)
	}
	a.EmitEvent("webview-config-changed")
	return nil
}

// SetWebViewPIN validates and persists the 4-digit kiosk PIN.
func (a *App) SetWebViewPIN(pin string) error {
	logger.Debug("Setting WebView PIN")
	return a.config.SetWebViewPIN(pin)
}

// ValidateWebViewPIN returns true when pin matches the stored PIN.
// The incoming value is compared but never logged.
func (a *App) ValidateWebViewPIN(pin string) bool {
	return a.config.CheckWebViewPIN(pin)
}

// NavigateToWebapp navigates the main Wails WebView directly to the configured webapp URL.
func (a *App) NavigateToWebapp(url string) {
	if url == "" {
		return
	}
	a.isWebappActive.Store(true)
	if a.webserver != nil {
		a.webserver.SetWebappActive(true)
	}
	fullscreen := a.config.GetWebViewEnabled()
	zoom := a.config.GetWebViewZoom()
	if fullscreen {
		a.SetWindowFullscreen(true)
	} else {
		a.SetWindowFullscreen(false)
	}
	if a.webserver != nil {
		a.webserver.PostWebviewAction("open", url, fullscreen, zoom)
	}
	if a.mainWindow != nil {
		menubar.ConfigureWebviewSettings()
		menubar.ApplyWebviewZoom(zoom)
		a.mainWindow.SetURL(url)
		a.mainWindow.ExecJS(fmt.Sprintf(`(function() {
			window.__eposReloading = false;
			try {
				window.location.replace(%q);
			} catch(e) {
				window.location.href = %q;
			}
		})();`, url, url))
		port := 4545
		if a.webserver != nil && a.webserver.Port > 0 {
			port = a.webserver.Port
		}
		gestureScript := cornerGestureJS(port, a.config.HasWebViewPIN())
		zoomScript := buildZoomJS(zoom, port, url)
		go func() {
			for _, delay := range []time.Duration{150 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 1500 * time.Millisecond, 3000 * time.Millisecond} {
				time.Sleep(delay)
				if a.isWebappActive.Load() && a.mainWindow != nil {
					a.mainWindow.ExecJS(gestureScript)
					a.mainWindow.ExecJS(zoomScript)
				}
			}
		}()
	}
	a.EmitEvent("webview-config-changed")
}

// NavigateToLocalUI navigates the main Wails WebView back to the local administration UI.
func (a *App) NavigateToLocalUI() {
	a.isWebappActive.Store(false)
	if a.webserver != nil {
		a.webserver.SetWebappActive(false)
		a.webserver.PostWebviewAction("close", "", false, 1.0)
	}
	a.SetWindowFullscreen(false)
	menubar.ApplyWebviewZoom(1.0)
	if a.mainWindow != nil {
		a.mainWindow.SetURL("/")
		go func() {
			time.Sleep(300 * time.Millisecond)
			if !a.isWebappActive.Load() && a.mainWindow != nil {
				port := 4545
				if a.webserver != nil && a.webserver.Port > 0 {
					port = a.webserver.Port
				}
				a.mainWindow.ExecJS(buildZoomJS(1.0, port, ""))
			}
		}()
	}
	a.EmitEvent("webview-config-changed")
}

func buildZoomJS(zoom float64, port int, targetUrl string) string {
	if zoom <= 0 {
		zoom = 1.0
	}
	if port <= 0 {
		port = 4545
	}
	return fmt.Sprintf(`(function() {
    var z = %f;
    var targetKioskUrl = %q;
    function applyZoom() {
        if ((window.location.hostname === '127.0.0.1' || window.location.hostname === 'localhost' || window.location.hostname === 'wails.localhost') && (window.location.pathname === '/' || window.location.pathname === '')) {
            return;
        }
        var styleId = '__epos_zoom_style__';
        var style = document.getElementById(styleId);
        if (!z || Math.abs(z - 1.0) < 0.001) {
            if (style && style.parentNode) {
                style.parentNode.removeChild(style);
            }
            if (document.documentElement) {
                document.documentElement.style.zoom = '';
                document.documentElement.style.minHeight = '';
                document.documentElement.style.minWidth = '';
                document.documentElement.style.height = '';
                document.documentElement.style.width = '';
            }
            if (document.body) {
                document.body.style.minHeight = '';
                document.body.style.height = '';
            }
            return;
        }
        if (!style) {
            style = document.createElement('style');
            style.id = styleId;
            var target = document.head || document.documentElement;
            if (target) {
                target.appendChild(style);
            }
        }
        var inv = (100.0 / z).toFixed(3);
        if (style) {
            style.textContent = 
                'html { ' +
                '  zoom: ' + z + ' !important; ' +
                '  min-height: ' + inv + 'vh !important; ' +
                '  min-width: ' + inv + 'vw !important; ' +
                '  height: ' + inv + 'vh !important; ' +
                '  width: ' + inv + 'vw !important; ' +
                '} ' +
                'body { ' +
                '  min-height: 100%% !important; ' +
                '  height: 100%% !important; ' +
                '} ' +
                '.pos, .point-of-sale, .o_action_manager, .o_web_client, #app, #root, main, [role="main"] { ' +
                '  min-height: 100%% !important; ' +
                '  height: 100%% !important; ' +
                '}';
        }
        if (document.documentElement) {
            document.documentElement.style.zoom = '' + z;
            document.documentElement.style.minHeight = inv + 'vh';
            document.documentElement.style.minWidth = inv + 'vw';
            document.documentElement.style.height = inv + 'vh';
            document.documentElement.style.width = inv + 'vw';
        }
        if (document.body) {
            document.body.style.minHeight = '100%%';
            document.body.style.height = '100%%';
        }
    }
    applyZoom();
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', applyZoom);
    }
    window.addEventListener('load', applyZoom);
    window.addEventListener('pageshow', applyZoom);
    var delays = [150, 400, 800, 1500, 3000];
    for (var i = 0; i < delays.length; i++) {
        setTimeout(applyZoom, delays[i]);
    }
    if (window.MutationObserver && !window.__eposZoomObserver) {
        try {
            window.__eposZoomObserver = new MutationObserver(function() {
                var s = document.getElementById('__epos_zoom_style__');
                if (!s) applyZoom();
            });
            window.__eposZoomObserver.observe(document.documentElement || document, { childList: true, subtree: true });
        } catch(e) {}
    }
    try {
        if (!window.__eposReloadInterceptorInstalled) {
            window.__eposReloadInterceptorInstalled = true;
            var lastTriggerTime = 0;
            function triggerReloadKiosk() {
                var now = Date.now();
                if (now - lastTriggerTime < 1500) return;
                lastTriggerTime = now;
                console.log('[EposProxy] triggerReloadKiosk invoked');

                // 1. Android native JavascriptInterface
                if (window._eposKiosk && typeof window._eposKiosk.requestReload === 'function') {
                    try { window._eposKiosk.requestReload(); } catch(e) {}
                }
                // 2. Linux WebKitGTK native message handler
                if (window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.eposKiosk) {
                    try { window.webkit.messageHandlers.eposKiosk.postMessage("reload"); } catch(e) {}
                }
                // 3. Instant in-page navigation if target URL is set and page has navigated away
                if (targetKioskUrl && window.location.href !== targetKioskUrl) {
                    try {
                        window.location.replace(targetKioskUrl);
                        return;
                    } catch(e) {}
                }
                // 4. HTTP fallback
                var reloadUrl = 'http://127.0.0.1:%d/api/webview/reload';
                try {
                    if (navigator.sendBeacon) {
                        navigator.sendBeacon(reloadUrl);
                    }
                } catch(e) {}
                try {
                    var xhr = new XMLHttpRequest();
                    xhr.open('POST', reloadUrl, true);
                    xhr.send();
                } catch(e) {}
                try {
                    var img = new Image();
                    img.src = reloadUrl + '?t=' + now;
                } catch(e) {}
            }

            // 1. Intercept location.reload
            try {
                if (window.Location && window.Location.prototype) {
                    window.Location.prototype.reload = function() {
                        triggerReloadKiosk();
                    };
                }
            } catch(e) {}

            // 2. Intercept history.go(0)
            try {
                if (window.history) {
                    var origGo = window.history.go;
                    window.history.go = function(delta) {
                        if (delta === 0 || delta === undefined) {
                            triggerReloadKiosk();
                            return;
                        }
                        if (origGo) return origGo.apply(this, arguments);
                    };
                }
            } catch(e) {}

            // 3. Intercept localStorage.clear() and sessionStorage.clear()
            try {
                if (window.Storage && window.Storage.prototype) {
                    var origStorageClear = window.Storage.prototype.clear;
                    window.Storage.prototype.clear = function() {
                        try {
                            if (origStorageClear) origStorageClear.apply(this, arguments);
                        } finally {
                            triggerReloadKiosk();
                        }
                    };
                }
            } catch(e) {}

            try {
                if (window.localStorage) {
                    var origLocalClear = window.localStorage.clear;
                    window.localStorage.clear = function() {
                        try {
                            if (origLocalClear) origLocalClear.apply(this, arguments);
                        } finally {
                            triggerReloadKiosk();
                        }
                    };
                }
            } catch(e) {}

            try {
                if (window.sessionStorage) {
                    var origSessionClear = window.sessionStorage.clear;
                    window.sessionStorage.clear = function() {
                        try {
                            if (origSessionClear) origSessionClear.apply(this, arguments);
                        } finally {
                            triggerReloadKiosk();
                        }
                    };
                }
            } catch(e) {}

            // 4. Intercept indexedDB.deleteDatabase
            try {
                if (window.indexedDB && window.indexedDB.deleteDatabase) {
                    var origDeleteDB = window.indexedDB.deleteDatabase;
                    window.indexedDB.deleteDatabase = function() {
                        var req = origDeleteDB.apply(this, arguments);
                        triggerReloadKiosk();
                        return req;
                    };
                }
            } catch(e) {}

            // 5. Check if current page was loaded via reload
            var navEntries = window.performance && window.performance.getEntriesByType && window.performance.getEntriesByType('navigation');
            var isReload = (navEntries && navEntries.length > 0 && navEntries[0].type === 'reload') ||
                           (window.performance && window.performance.navigation && window.performance.navigation.type === 1);
            if (isReload) {
                triggerReloadKiosk();
            }

            // 6. Self-heal Odoo device_identifier sequence if localStorage was cleared
            try {
                var origGetItem = Storage.prototype.getItem;
                Storage.prototype.getItem = function(key) {
                    var val = origGetItem.apply(this, arguments);
                    if ((!val || val === 'null') && typeof key === 'string' && key.indexOf('unique_device_identifier') !== -1) {
                        var fallback = JSON.stringify({
                            device_identifier: 'kiosk_device_' + (window.crypto && window.crypto.randomUUID ? window.crypto.randomUUID() : Date.now()),
                            next_number: 1,
                            unsynced_number_stack: []
                        });
                        try { this.setItem(key, fallback); } catch(e) {}
                        return fallback;
                    }
                    return val;
                };
            } catch(e) {}

            // 7. Intercept console.error for fatal Odoo initialization errors
            try {
                var origConsoleErr = console.error;
                console.error = function() {
                    try {
                        var str = Array.prototype.slice.call(arguments).map(String).join(' ').toLowerCase();
                        if (str.indexOf('device_identifier') !== -1 || str.indexOf('indexeddb db is null') !== -1) {
                            triggerReloadKiosk();
                        }
                    } catch(e) {}
                    if (origConsoleErr) origConsoleErr.apply(console, arguments);
                };
            } catch(e) {}

            // 8. Click interception on reload/retry/close buttons in error dialogs
            document.addEventListener('click', function(e) {
                var el = e.target;
                while (el && el !== document) {
                    var txt = (el.innerText || el.textContent || '').trim().toLowerCase();
                    if (txt === 'reload' || txt === 'refresh' || txt === 'recharger' || txt === 'close' || txt === 'fermer') {
                        if (el.closest && (el.closest('.o_error_dialog') || el.closest('.modal') || el.closest('[role="dialog"]'))) {
                            setTimeout(triggerReloadKiosk, 50);
                            break;
                        }
                    }
                    el = el.parentElement;
                }
            }, true);

            // 9. Auto-detect and recover from error modals on screen
            try {
                setInterval(function() {
                    var modals = document.querySelectorAll('.o_error_dialog, .o_dialog_error, .modal.show, [role="dialog"]');
                    for (var i = 0; i < modals.length; i++) {
                        var m = modals[i];
                        var txt = (m.innerText || m.textContent || '').toLowerCase();
                        if (txt.indexOf('something went wrong') !== -1 || txt.indexOf('oops!') !== -1 || txt.indexOf('technical details') !== -1) {
                            triggerReloadKiosk();
                            break;
                        }
                    }
                }, 1000);
            } catch(e) {}

            // 10. Auto-recover from fatal Odoo session initialization errors
            window.addEventListener('error', function(e) {
                var msg = (e && e.message) ? e.message.toLowerCase() : '';
                if (msg.includes('device_identifier') || 
                    msg.includes('pos_session') ||
                    (msg.includes('indexeddb') && msg.includes('null')) ||
                    msg.includes('cannot read properties of null') ||
                    msg.includes('cannot read properties of undefined')) {
                    triggerReloadKiosk();
                }
            });
        }
    } catch(e) {}
})();`, zoom, targetUrl, port)
}

// SetWebViewEnabled persists the lockdown (fullscreen kiosk) mode flag and toggles fullscreen if webapp is active.
func (a *App) SetWebViewEnabled(v bool) error {
	logger.Debugf("Setting WebView lockdown enabled: %v", v)
	if err := a.config.SetWebViewEnabled(v); err != nil {
		return err
	}
	if a.isWebappActive.Load() {
		a.SetWindowFullscreen(v)
	}
	if a.webserver != nil {
		a.webserver.PostWebviewAction("lockdown", "", v, 1.0)
	}
	a.EmitEvent("webview-config-changed")
	return nil
}

// SetWindowFullscreen puts the main Wails window into or out of fullscreen
// and hides/restores the native menu bar accordingly.
func (a *App) SetWindowFullscreen(fullscreen bool) {
	if a.mainWindow != nil {
		if fullscreen {
			a.mainWindow.Fullscreen()
			a.mainWindow.HideMenuBar()
			menubar.SetNativeMenubarVisible(false)
			menubar.SetNativeFullscreen(true)
			go func() {
				for _, delay := range []time.Duration{100 * time.Millisecond, 300 * time.Millisecond, 800 * time.Millisecond} {
					time.Sleep(delay)
					if a.isWebappActive.Load() || a.config.GetWebViewEnabled() {
						menubar.SetNativeMenubarVisible(false)
						menubar.SetNativeFullscreen(true)
					}
				}
			}()
		} else {
			a.mainWindow.UnFullscreen()
			a.mainWindow.ShowMenuBar()
			menubar.SetNativeMenubarVisible(true)
			menubar.SetNativeFullscreen(false)
		}
	}
}

// ReloadKiosk reloads the active top-level webview with the configured whole webapp URL.
func (a *App) ReloadKiosk() {
	if a.mainWindow != nil {
		url := ""
		if a.config != nil {
			url = a.config.GetWebViewURL()
		}
		if url != "" {
			a.NavigateToWebapp(url)
		} else {
			a.mainWindow.Reload()
		}
	}
	if a.webserver != nil {
		a.webserver.PostWebviewAction("reload", "", false, 1.0)
	}
	a.EmitEvent("kiosk-reload")
}

// QuitApp exits the application cleanly
func (a *App) QuitApp() {
	logger.Info("QuitApp called, shutting down application")
	if app := application.Get(); app != nil {
		app.Quit()
	}
}

func (a *App) ConfirmRemoveLANPrinter(ip string) (bool, error) {
	logger.Debugf("Remove LAN printer requested: %s", ip)

	confirmed := a.dlg().Question("Remove Printer", fmt.Sprintf("Are you sure you want to remove the printer at %s?", ip))

	if confirmed {
		if err := a.config.RemoveLANPrinter(ip); err != nil {
			return false, fmt.Errorf("failed to remove LAN printer: %w", err)
		}
		return true, nil
	}

	logger.Infof("Remove LAN printer cancelled by user")
	return false, nil
}

func (a *App) CheckLANPrinterStatus(ip string) bool {
	logger.Debugf("Checking LAN printer status: %s", ip)
	return printer.CheckLANPrinter(ip) == nil
}

func (a *App) DownloadLogs() {
	logger.Debugf("Download logs requested")
	logDir := logger.LogDirectory()
	zipName := fmt.Sprintf("epos-proxy-logs-%s.zip", time.Now().Format("2006-01-02"))
	logger.Debugf("Creating logs archive: %s", zipName)

	savePath, err := a.dlg().SaveFile(&application.SaveFileDialogOptions{
		Title:    "Save Archive",
		Filename: zipName,
		Filters: []application.FileFilter{
			{
				DisplayName: "Zip Archives (*.zip)",
				Pattern:     "*.zip",
			},
		},
	})

	if err != nil {
		logger.Errorf("Save dialog failed: %v", err)
		a.showError("Download Logs Failed", err.Error())
		return
	}

	// An empty path means the user dismissed the save dialog.
	if savePath == "" {
		logger.Infof("Download logs cancelled by user")
		return
	}

	if err := util.ZipLogs(logDir, savePath); err != nil {
		logger.Errorf("Log export failed: %v", err)
		a.showError("Download Logs Failed", err.Error())
		return
	}
	logger.Infof("Logs successfully exported to: %s", savePath)
}

func (a *App) IsAutostartEnabled() bool {
	return a.autoStart.IsEnabled()
}

func (a *App) EnableAutostart() error {
	logger.Info("Enabling autostart")

	if runtime.GOOS == "linux" {
		return util.EnableLinuxAutostart()
	}

	if !a.autoStart.IsEnabled() {
		return a.autoStart.Enable()
	}

	return nil
}

func (a *App) DisableAutostart() error {
	logger.Info("Disabling autostart")

	if a.autoStart.IsEnabled() {
		return a.autoStart.Disable()
	}

	return nil
}

func (a *App) SetNetworkPrintingEnabled(enabled bool) error {
	logger.Infof("Setting network printing enabled: %v", enabled)
	return a.config.SetNetworkPrintingEnabled(enabled)
}

func (a *App) IsNetworkPrintingEnabled() bool {
	if a.config == nil {
		return false
	}
	return a.config.IsNetworkPrintingEnabled()
}

type TroubleshootInfo struct {
	ActiveFirewall string `json:"activeFirewall"`
	FirewallZone   string `json:"firewallZone"`
	Port           int    `json:"port"`
	Subnet         string `json:"subnet"`
	LocalIP        string `json:"localIp"`
	ExecPath       string `json:"execPath"`
}

func (a *App) GetTroubleshootInfo() TroubleshootInfo {
	netInfo := util.GetNetworkInfo()
	execPath, _ := os.Executable()
	return TroubleshootInfo{
		ActiveFirewall: netInfo.ActiveFirewall,
		FirewallZone:   netInfo.Zone,
		Port:           a.config.GetPort(),
		Subnet:         netInfo.Subnet,
		LocalIP:        netInfo.IP,
		ExecPath:       execPath,
	}
}
