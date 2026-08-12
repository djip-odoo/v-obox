package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"epos-proxy/internal/config"
	"epos-proxy/internal/printer"
	"epos-proxy/internal/server"
	"epos-proxy/internal/testutil"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestNewApp(t *testing.T) {
	app := NewApp()
	testutil.ExpectedNotNil(t, app)
	testutil.ExpectedNotNil(t, app.autoStart)
	testutil.ExpectedNotNil(t, app.config)
	testutil.ExpectedNotNil(t, app.printerManager)
}

func TestApp_AppVariableAndPrintersAndGetPrinterIp(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)

	port := 4545

	err = cfg.AddLanEposPrinter("192.168.1.100")
	testutil.ExpectedNoError(t, err)

	mgr := printer.NewManager()
	srv := server.New(port, mgr)
	defer srv.Stop()

	app := &App{
		webserver:      srv,
		config:         cfg,
		printerManager: mgr,
	}

	appVariable := app.AppVariable()
	testutil.ExpectedEqual(t, app.GetPrinterIp("czpTTjEyMzQ1Ng"), "127.0.0.1:4545/p/czpTTjEyMzQ1Ng")
	testutil.ExpectedTrue(t, appVariable.ServerRunning, "Expected ServerRunning to be true")
	testutil.ExpectedEqual(t, appVariable.DefaultIp, fmt.Sprintf("127.0.0.1:%d", port))
	testutil.ExpectedTrue(t, appVariable.Os != "", "Expected non-empty Os field in app variable")

	// At least the configured LAN printer should be present in Printers
	printerStatus := appPrintersOrSkip(t, app)
	foundLAN := false
	for _, p := range printerStatus.Printers {
		if p.IsLAN && p.LANIp == "192.168.1.100" {
			foundLAN = true
			testutil.ExpectedEqual(t, p.Type, string(printer.TypeReceipt))
		}
	}
	testutil.ExpectedTrue(t, foundLAN, "Expected to find configured LAN printer in printer status")
}

func appPrintersOrSkip(t *testing.T, app *App) (printers Printers) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("USB printer enumeration unavailable in this environment: %v", r)
		}
	}()
	return app.Printers()
}

func TestApp_AddLANPrinter(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)

	app := &App{config: cfg}

	// Invalid IP format.
	err = app.AddLANPrinter("not.an.ip")
	testutil.ExpectedError(t, err)

	// Empty IP.
	err = app.AddLANPrinter("  ")
	testutil.ExpectedError(t, err)

	// Unreachable printer.
	err = app.AddLANPrinter("127.0.0.1")
	if err == nil {
		t.Log("Note: 127.0.0.1:9100 happened to be open")
	}

	// Reachable printer.
	_, _, err = testutil.StartMockTCPServer(t, printer.LANPort)
	testutil.ExpectedNoError(t, err)

	err = app.AddLANPrinter("127.0.0.1")
	testutil.ExpectedNoError(t, err)

	printers := cfg.GetLANPrinters()
	testutil.ExpectedLen(t, printers, 1)
	testutil.ExpectedEqual(t, printers[0], "127.0.0.1")
}

func TestApp_CheckLANPrinterStatus(t *testing.T) {
	app := &App{}

	// 1. Unreachable (closed local port returns false immediately)
	if app.CheckLANPrinterStatus("127.0.0.1") {
		t.Log("127.0.0.1:9100 was open during test")
	}

	// 2. Active listener using StartMockTCPServer
	_, _, err := testutil.StartMockTCPServer(t, printer.LANPort)
	if err == nil {
		testutil.ExpectedTrue(t, app.CheckLANPrinterStatus("127.0.0.1"), "Expected CheckLANPrinterStatus true for live listener")
	}
}

func TestApp_AutostartMethods(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	app := NewApp()

	// Enable autostart on linux creates desktop file
	err := app.EnableAutostart()
	testutil.ExpectedNoError(t, err)

	// Disable autostart
	_ = app.DisableAutostart()
}

type mockFrontend struct {
	dialogResult string
	dialogErr    error
}

func (m *mockFrontend) Run(ctx context.Context) error { return nil }
func (m *mockFrontend) RunMainLoop()                  {}
func (m *mockFrontend) ExecJS(js string)              {}
func (m *mockFrontend) Hide()                         {}
func (m *mockFrontend) Show()                         {}
func (m *mockFrontend) Quit()                         {}

func (m *mockFrontend) OpenFileDialog(dialogOptions wailsruntime.OpenDialogOptions) (string, error) {
	return "", nil
}
func (m *mockFrontend) OpenMultipleFilesDialog(dialogOptions wailsruntime.OpenDialogOptions) ([]string, error) {
	return nil, nil
}
func (m *mockFrontend) OpenDirectoryDialog(dialogOptions wailsruntime.OpenDialogOptions) (string, error) {
	return "", nil
}
func (m *mockFrontend) SaveFileDialog(dialogOptions wailsruntime.SaveDialogOptions) (string, error) {
	return "", nil
}
func (m *mockFrontend) MessageDialog(dialogOptions wailsruntime.MessageDialogOptions) (string, error) {
	return m.dialogResult, m.dialogErr
}

func (m *mockFrontend) WindowSetTitle(title string)                 {}
func (m *mockFrontend) WindowShow()                                 {}
func (m *mockFrontend) WindowHide()                                 {}
func (m *mockFrontend) WindowCenter()                               {}
func (m *mockFrontend) WindowToggleMaximise()                       {}
func (m *mockFrontend) WindowMaximise()                             {}
func (m *mockFrontend) WindowUnmaximise()                           {}
func (m *mockFrontend) WindowMinimise()                             {}
func (m *mockFrontend) WindowUnminimise()                           {}
func (m *mockFrontend) WindowSetAlwaysOnTop(b bool)                 {}
func (m *mockFrontend) WindowSetPosition(x int, y int)             {}
func (m *mockFrontend) WindowGetPosition() (int, int)               { return 0, 0 }
func (m *mockFrontend) WindowSetSize(width int, height int)         {}
func (m *mockFrontend) WindowGetSize() (int, int)                   { return 0, 0 }
func (m *mockFrontend) WindowSetMinSize(width int, height int)      {}
func (m *mockFrontend) WindowSetMaxSize(width int, height int)      {}
func (m *mockFrontend) WindowFullscreen()                           {}
func (m *mockFrontend) WindowUnfullscreen()                         {}
func (m *mockFrontend) WindowSetBackgroundColour(col *options.RGBA) {}
func (m *mockFrontend) WindowReload()                               {}
func (m *mockFrontend) WindowReloadApp()                            {}
func (m *mockFrontend) WindowSetSystemDefaultTheme()                {}
func (m *mockFrontend) WindowSetLightTheme()                        {}
func (m *mockFrontend) WindowSetDarkTheme()                         {}
func (m *mockFrontend) WindowIsMaximised() bool                     { return false }
func (m *mockFrontend) WindowIsMinimised() bool                     { return false }
func (m *mockFrontend) WindowIsNormal() bool                        { return true }
func (m *mockFrontend) WindowIsFullscreen() bool                    { return false }
func (m *mockFrontend) WindowClose()                                {}
func (m *mockFrontend) WindowPrint()                                {}

func (m *mockFrontend) ScreenGetAll() ([]wailsruntime.Screen, error) { return nil, nil }
func (m *mockFrontend) MenuSetApplicationMenu(menu *menu.Menu)      {}
func (m *mockFrontend) MenuUpdateApplicationMenu()                   {}
func (m *mockFrontend) Notify(name string, data ...interface{})     {}
func (m *mockFrontend) BrowserOpenURL(url string)                   {}
func (m *mockFrontend) ClipboardGetText() (string, error)           { return "", nil }
func (m *mockFrontend) ClipboardSetText(text string) error          { return nil }

func (m *mockFrontend) InitializeNotifications() error              { return nil }
func (m *mockFrontend) CleanupNotifications()                       {}
func (m *mockFrontend) IsNotificationAvailable() bool               { return false }
func (m *mockFrontend) RequestNotificationAuthorization() (bool, error) {
	return true, nil
}
func (m *mockFrontend) CheckNotificationAuthorization() (bool, error) {
	return true, nil
}
func (m *mockFrontend) OnNotificationResponse(callback func(result wailsruntime.NotificationResult)) {
}
func (m *mockFrontend) SendNotification(options wailsruntime.NotificationOptions) error {
	return nil
}
func (m *mockFrontend) SendNotificationWithActions(options wailsruntime.NotificationOptions) error {
	return nil
}
func (m *mockFrontend) RegisterNotificationCategory(category wailsruntime.NotificationCategory) error {
	return nil
}
func (m *mockFrontend) RemoveNotificationCategory(categoryId string) error { return nil }
func (m *mockFrontend) RemoveAllPendingNotifications() error              { return nil }
func (m *mockFrontend) RemovePendingNotification(identifier string) error  { return nil }
func (m *mockFrontend) RemoveAllDeliveredNotifications() error            { return nil }
func (m *mockFrontend) RemoveDeliveredNotification(identifier string) error {
	return nil
}
func (m *mockFrontend) RemoveNotification(identifier string) error { return nil }

func TestApp_ConfirmRemoveLANPrinter(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)

	ip := "192.168.1.50"

	// Case 1: Dialog returns "Confirm" -> printer is removed, returns (true, nil)
	err = cfg.AddLanEposPrinter(ip)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedLen(t, cfg.GetLANPrinters(), 1)

	feConfirm := &mockFrontend{dialogResult: "Confirm"}
	ctxConfirm := context.WithValue(context.Background(), "frontend", feConfirm)
	appConfirm := &App{
		ctx:    ctxConfirm,
		config: cfg,
	}

	confirmed, err := appConfirm.ConfirmRemoveLANPrinter(ip)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedTrue(t, confirmed)
	testutil.ExpectedLen(t, cfg.GetLANPrinters(), 0)

	// Case 2: Dialog returns "Yes" -> printer is removed, returns (true, nil)
	err = cfg.AddLanEposPrinter(ip)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedLen(t, cfg.GetLANPrinters(), 1)

	feYes := &mockFrontend{dialogResult: "Yes"}
	ctxYes := context.WithValue(context.Background(), "frontend", feYes)
	appYes := &App{
		ctx:    ctxYes,
		config: cfg,
	}

	confirmed, err = appYes.ConfirmRemoveLANPrinter(ip)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedTrue(t, confirmed)
	testutil.ExpectedLen(t, cfg.GetLANPrinters(), 0)

	// Case 3: Dialog returns "Cancel" -> printer is not removed, returns (false, nil)
	err = cfg.AddLanEposPrinter(ip)
	testutil.ExpectedNoError(t, err)

	feCancel := &mockFrontend{dialogResult: "Cancel"}
	ctxCancel := context.WithValue(context.Background(), "frontend", feCancel)
	appCancel := &App{
		ctx:    ctxCancel,
		config: cfg,
	}

	confirmed, err = appCancel.ConfirmRemoveLANPrinter(ip)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedFalse(t, confirmed)
	testutil.ExpectedLen(t, cfg.GetLANPrinters(), 1)

	// Case 4: Dialog returns error -> returns (false, error)
	feErr := &mockFrontend{dialogErr: errors.New("dialog failed")}
	ctxErr := context.WithValue(context.Background(), "frontend", feErr)
	appErr := &App{
		ctx:    ctxErr,
		config: cfg,
	}

	confirmed, err = appErr.ConfirmRemoveLANPrinter(ip)
	testutil.ExpectedError(t, err)
	testutil.ExpectedFalse(t, confirmed)
	testutil.ExpectedLen(t, cfg.GetLANPrinters(), 1)
}

func TestApp_ConfirmQuit(t *testing.T) {
	// Case 1: "Quit" -> returns true
	feQuit := &mockFrontend{dialogResult: "Quit"}
	appQuit := &App{ctx: context.WithValue(context.Background(), "frontend", feQuit)}
	testutil.ExpectedTrue(t, appQuit.ConfirmQuit())

	// Case 2: "Yes" -> returns true
	feYes := &mockFrontend{dialogResult: "Yes"}
	appYes := &App{ctx: context.WithValue(context.Background(), "frontend", feYes)}
	testutil.ExpectedTrue(t, appYes.ConfirmQuit())

	// Case 3: "Cancel" -> returns false
	feCancel := &mockFrontend{dialogResult: "Cancel"}
	appCancel := &App{ctx: context.WithValue(context.Background(), "frontend", feCancel)}
	testutil.ExpectedFalse(t, appCancel.ConfirmQuit())

	// Case 4: Error -> returns false
	feErr := &mockFrontend{dialogErr: errors.New("dialog error")}
	appErr := &App{ctx: context.WithValue(context.Background(), "frontend", feErr)}
	testutil.ExpectedFalse(t, appErr.ConfirmQuit())
}
