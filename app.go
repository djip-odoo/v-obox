package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"
	"epos-proxy/internal/server"
	"epos-proxy/internal/util"

	autostart "github.com/emersion/go-autostart"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// dialoger abstracts the Wails runtime dialog calls. Production code uses
// runtimeDialogs; tests substitute a fake so the dialog-driven code paths can
// be exercised without a live Wails context.
type dialoger interface {
	Message(ctx context.Context, opts wailsruntime.MessageDialogOptions) (string, error)
	SaveFile(ctx context.Context, opts wailsruntime.SaveDialogOptions) (string, error)
}

// runtimeDialogs forwards to the real Wails runtime.
type runtimeDialogs struct{}

func (runtimeDialogs) Message(ctx context.Context, opts wailsruntime.MessageDialogOptions) (string, error) {
	return wailsruntime.MessageDialog(ctx, opts)
}

func (runtimeDialogs) SaveFile(ctx context.Context, opts wailsruntime.SaveDialogOptions) (string, error) {
	return wailsruntime.SaveFileDialog(ctx, opts)
}

// App struct
type App struct {
	ctx            context.Context
	webserver      *server.Server
	config         *config.Manager
	printerManager *printer.Manager
	autoStart      *autostart.App
	dialogs        dialoger
	appID          string
}

// dlg returns the dialog backend, defaulting to the Wails runtime so an App
// built as a bare struct literal still behaves correctly.
func (a *App) dlg() dialoger {
	if a.dialogs == nil {
		return runtimeDialogs{}
	}
	return a.dialogs
}

// showError surfaces an error to the user and logs any failure to do so.
func (a *App) showError(title, message string) {
	if _, err := a.dlg().Message(a.ctx, wailsruntime.MessageDialogOptions{
		Type:    wailsruntime.ErrorDialog,
		Title:   title,
		Message: message,
	}); err != nil {
		logger.Errorf("Failed to show error dialog %q: %v", title, err)
	}
}

type Printer struct {
	Name       string `json:"name"`
	Ip         string `json:"ip"`
	Identifier string `json:"identifier"`
	IsLAN      bool   `json:"isLAN"`
	LANIp      string `json:"lanIp,omitempty"`
	Online     bool   `json:"online"`
	Type       string `json:"type"`
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
	AppID         string `json:"appId,omitempty"`
}

type OdooStatusInterface struct {
	AppId           string `json:"appId"`
	IpAddress       string `json:"ipAddress"`
	Connected       bool   `json:"connected"`
	DbURL           string `json:"dbUrl"`
	WebsocketStatus string `json:"websocketStatus"`
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
	a.dialogs = runtimeDialogs{}

	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	logger.Debugf("Application startup")

	cfg, err := config.NewManager()
	if err != nil {
		logger.Fatalf("Config initialization failed: %v", err)
	}

	if err := cfg.Load(); err != nil {
		logger.Warnf("Config load warning: %v", err)
	}

	a.appID = cfg.GetAppID()
	logger.Infof("Application ID: %s", a.appID)

	logger.Debugf("Config loaded from %s", cfg.Path())

	a.config = cfg

	port, err := cfg.ResolvePort()
	if err != nil {
		logger.Warn("Unable to resolve port, using default")
	}

	a.webserver = server.New(port, a.printerManager, a.config)
	a.webserver.OnStatusChange(func(st server.OdooStatus) {
		status := a.CheckOdooStatus()
		wailsruntime.EventsEmit(a.ctx, "odoo:status_changed", status)
	})
}

func (a *App) shutdown(ctx context.Context) {
	logger.Infof("Stopping proxy server")

	if err := a.webserver.Stop(); err != nil {
		logger.Errorf("Server stop error: %v", err)
	}
}

func (a *App) AppVariable() AppVariable {
	return AppVariable{
		Os:            runtime.GOOS,
		ServerRunning: a.webserver.Running(),
		AppID:         a.appID,
	}
}

func (a *App) GetPrinterIp(id string) string {
	ip := fmt.Sprintf("127.0.0.1:%d/p/%s", a.webserver.Port, id)
	logger.Debugf("Generated printer endpoint: %s", ip)
	return ip
}

func (a *App) Printers() Printers {
	logger.Debug("Collecting printer status")

	discovered := printer.DiscoverAllPrinters(a.config)
	printers := make([]Printer, 0, len(discovered.Available))
	unavailablePrinters := make([]UnavailablePrinter, 0, len(discovered.Unavailable))
	errorMsg := ""

	if discovered.Error != nil {
		errorMsg = discovered.Error.Error()
	}

	for _, p := range discovered.Available {
		printers = append(printers, Printer{
			Identifier: p.Identifier,
			Name:       p.Name,
			Ip:         a.GetPrinterIp(p.Identifier),
			IsLAN:      p.IsLAN,
			LANIp:      p.LANIp,
			Online:     p.Online,
			Type:       string(p.Type),
		})
	}

	for _, u := range discovered.Unavailable {
		unavailablePrinters = append(unavailablePrinters, UnavailablePrinter{
			Name:     u.Name,
			ErrorMsg: u.Error,
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

func (a *App) ConfirmRemoveLANPrinter(ip string) (bool, error) {

	logger.Debugf("Remove LAN printer requested: %s", ip)

	result, err := a.dlg().Message(a.ctx, wailsruntime.MessageDialogOptions{
		Type:          wailsruntime.QuestionDialog,
		Title:         "Remove Printer",
		Message:       fmt.Sprintf("Are you sure you want to remove the printer at %s?", ip),
		Buttons:       []string{"Cancel", "Confirm"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	if err != nil {
		return false, fmt.Errorf("failed to show confirmation dialog: %w", err)
	}
	if result == "Confirm" || result == "Yes" {
		if err := a.config.RemoveLANPrinter(ip); err != nil {
			return false, fmt.Errorf("failed to remove LAN printer: %w", err)
		}
		return true, nil
	}
	logger.Infof("Remove LAN printer cancelled, Remove printer dialog result: %s", result)
	return false, nil
}

func (a *App) CheckLANPrinterStatus(ip string) bool {
	logger.Debugf("Checking LAN printer status: %s", ip)
	return printer.CheckLANPrinter(ip) == nil
}

func (a *App) DownloadLogs() {
	logger.Debugf("Download logs requested")
	logDir := logger.LogDirectory()
	zipName := fmt.Sprintf("epos-proxy-logs-%s.zip",
		time.Now().Format("2006-01-02"))
	logger.Debugf("Creating logs archive: %s", zipName)
	savePath, err := a.dlg().SaveFile(a.ctx, wailsruntime.SaveDialogOptions{
		Title:           "Save Archive",
		DefaultFilename: zipName,
		Filters: []wailsruntime.FileFilter{
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

func (a *App) CheckOdooStatus() OdooStatusInterface {
	logger.Debugf("checking Odoo status")

	status := OdooStatusInterface{
		AppId:           a.appID,
		WebsocketStatus: "disconnected",
		IpAddress:       fmt.Sprintf("127.0.0.1:%d", a.webserver.Port),
	}

	if a.webserver != nil {
		odooStatus := a.webserver.GetOdooStatus()
		status.Connected = odooStatus.Connected
		status.DbURL = odooStatus.DbURL
		status.WebsocketStatus = odooStatus.WebsocketStatus
		return status
	}

	if a.config != nil && a.config.HasOdooCredentials() {
		status.Connected = true
		status.DbURL = a.config.GetOdooDbURL()
		status.WebsocketStatus = "connected"
	}

	return status
}

func (a *App) DisconnectOdoo() error {
	logger.Infof("Disconnect Odoo requested")

	if a.webserver != nil {
		a.webserver.DisconnectOdoo()
	}

	if a.config != nil {
		if err := a.config.ClearOdooConfig(); err != nil {
			logger.Warnf("Failed to clear Odoo config: %v", err)
			return err
		}
	}

	wailsruntime.EventsEmit(a.ctx, "odoo:status_changed", a.CheckOdooStatus())
	return nil
}

func (a *App) ConfirmDisconnectOdoo() (bool, error) {
	logger.Debugf("Confirm Disconnect Odoo requested")

	result, err := wailsruntime.MessageDialog(a.ctx, wailsruntime.MessageDialogOptions{
		Type:          wailsruntime.QuestionDialog,
		Title:         "Disconnect Odoo",
		Message:       "Are you sure you want to disconnect and remove the Odoo database connection?",
		Buttons:       []string{"Cancel", "Disconnect"},
		DefaultButton: "Cancel",
		CancelButton:  "Cancel",
	})
	if err != nil {
		return false, fmt.Errorf("failed to show confirmation dialog: %w", err)
	}

	if result != "Disconnect" && result != "Confirm" && result != "Yes" {
		return false, nil
	}

	if err := a.DisconnectOdoo(); err != nil {
		return false, fmt.Errorf("failed to disconnect Odoo: %w", err)
	}

	return true, nil
}
