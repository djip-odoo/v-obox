package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"runtime"
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
	URL     string  `json:"url"`
	Enabled bool    `json:"enabled"`
	HasPIN  bool    `json:"hasPIN"`
	Zoom    float64 `json:"zoom"`
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
		if a.mainWindow != nil {
			if enabled {
				url := a.config.GetWebViewURL()
				if url != "" {
					a.SetWindowFullscreen(true)
					a.NavigateToWebapp(url)
				}
			} else {
				a.SetWindowFullscreen(false)
				a.NavigateToLocalUI()
			}
		}
	})
	a.webserver.SetConfigCallback(func() {
		a.EmitEvent("webview-config-changed")
	})
	a.webserver.SetKioskReloadCallback(func() {
		a.ReloadKiosk()
	})

	menubar.RegisterKioskExitGesture(func() {
		logger.Infof("Native kiosk exit gesture triggered")
		_ = a.SetWebViewEnabled(false)
	})
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
		URL:     a.config.GetWebViewURL(),
		Enabled: a.config.GetWebViewEnabled(),
		HasPIN:  a.config.HasWebViewPIN(),
		Zoom:    a.config.GetWebViewZoom(),
	}
}

// SetWebViewURL persists the kiosk URL.
func (a *App) SetWebViewURL(url string) error {
	logger.Debugf("Setting WebView URL")
	return a.config.SetWebViewURL(url)
}

// SetWebViewZoom persists the kiosk display zoom level.
func (a *App) SetWebViewZoom(zoom float64) error {
	logger.Debugf("Setting WebView zoom: %v", zoom)
	if err := a.config.SetWebViewZoom(zoom); err != nil {
		return err
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
	if a.mainWindow != nil && url != "" {
		if a.config.GetWebViewEnabled() {
			a.SetWindowFullscreen(true)
		} else {
			a.SetWindowFullscreen(false)
		}
		a.mainWindow.SetURL(url)
		a.mainWindow.ExecJS(fmt.Sprintf("window.location.href = %q;", url))
	}
}

// NavigateToLocalUI navigates the main Wails WebView back to the local administration UI.
func (a *App) NavigateToLocalUI() {
	if a.mainWindow != nil {
		a.SetWindowFullscreen(false)
		a.mainWindow.SetURL("/")
		a.mainWindow.ExecJS("window.location.href = '/';")
	}
}

// SetWebViewEnabled persists the kiosk-enabled flag and navigates the top-level WebView.
func (a *App) SetWebViewEnabled(v bool) error {
	logger.Debugf("Setting WebView enabled: %v", v)
	if err := a.config.SetWebViewEnabled(v); err != nil {
		return err
	}
	if a.mainWindow != nil {
		if v {
			url := a.config.GetWebViewURL()
			if url != "" {
				a.SetWindowFullscreen(true)
				a.NavigateToWebapp(url)
			}
		} else {
			a.SetWindowFullscreen(false)
			a.NavigateToLocalUI()
		}
	}
	a.EmitEvent("kiosk-state-changed", v)
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
		} else {
			a.mainWindow.UnFullscreen()
			a.mainWindow.ShowMenuBar()
			menubar.SetNativeMenubarVisible(true)
		}
	}
}

// ReloadKiosk reloads the active top-level webview.
func (a *App) ReloadKiosk() {
	if a.mainWindow != nil {
		a.mainWindow.Reload()
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
