package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"
	"epos-proxy/internal/server"
	"epos-proxy/internal/testutil"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// fakeDialogs is a dialoger that returns canned responses and records every
// invocation, so dialog-driven code paths can be tested without Wails.
type fakeDialogs struct {
	messageResult string
	messageErr    error
	savePath      string
	saveErr       error

	messages []wailsruntime.MessageDialogOptions
	saves    []wailsruntime.SaveDialogOptions
}

func (f *fakeDialogs) Message(_ context.Context, opts wailsruntime.MessageDialogOptions) (string, error) {
	f.messages = append(f.messages, opts)
	return f.messageResult, f.messageErr
}

func (f *fakeDialogs) SaveFile(_ context.Context, opts wailsruntime.SaveDialogOptions) (string, error) {
	f.saves = append(f.saves, opts)
	return f.savePath, f.saveErr
}

type emittedEvent struct {
	Name string
	Data []interface{}
}

type fakeEvents struct {
	emitted []emittedEvent
}

func (f *fakeEvents) Emit(_ context.Context, eventName string, optionalData ...interface{}) {
	f.emitted = append(f.emitted, emittedEvent{
		Name: eventName,
		Data: optionalData,
	})
}

func createTestApp(t testing.TB, cfg *config.Manager) *App {
	t.Helper()
	if cfg == nil {
		t.Setenv("HOME", t.TempDir())
		var err error
		cfg, err = config.NewManager()
		testutil.ExpectedNoError(t, err)
	}
	if cfg.Data.Port == 0 {
		cfg.Data.Port = testutil.GetFreePort(t)
	}
	mgr := printer.NewManager()
	srv := server.New(cfg.Data.Port, mgr, cfg)
	t.Cleanup(func() { _ = srv.Stop() })
	return &App{
		webserver:      srv,
		config:         cfg,
		printerManager: mgr,
		dialogs:        &fakeDialogs{},
		events:         &fakeEvents{},
	}
}

func TestNewApp(t *testing.T) {
	app := NewApp()
	testutil.ExpectedNotNil(t, app)
	testutil.ExpectedNotNil(t, app.autoStart)
	testutil.ExpectedNotNil(t, app.events)
	testutil.ExpectedNotNil(t, app.printerManager)
}

func TestApp_Startup_Success(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	port := testutil.GetFreePort(t)
	cfgFile := filepath.Join(tempDir, ".config", config.AppName, "config.json")
	_ = os.MkdirAll(filepath.Dir(cfgFile), 0755)
	_ = os.WriteFile(cfgFile, []byte(fmt.Sprintf(`{"port": %d}`, port)), 0644)

	dialogs := &fakeDialogs{}
	events := &fakeEvents{}
	app := NewApp()
	app.dialogs = dialogs
	app.events = events

	app.startup(context.Background())
	t.Cleanup(func() {
		if app.webserver != nil {
			_ = app.webserver.Stop()
		}
	})

	testutil.ExpectedNotNil(t, app.webserver)
	testutil.ExpectedTrue(t, app.webserver.Running(), "expected webserver to be running")
	testutil.ExpectedEqual(t, len(dialogs.messages), 0)

	status := app.CheckOdooStatus()
	testutil.ExpectedEqual(t, status.WebsocketStatus, "disconnected")
	testutil.ExpectedEqual(t, status.IpAddress, app.webserver.LocalAddr())
}

func TestApp_CheckOdooStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)
	_ = cfg.SetOdooCredentials("http://127.0.0.1:8069", "tok", "uuid-1")

	app := createTestApp(t, cfg)
	status := app.CheckOdooStatus()
	testutil.ExpectedEqual(t, status.DbURL, "http://127.0.0.1:8069")
	testutil.ExpectedEqual(t, status.AppId, cfg.GetAppID())
}

func TestApp_ConfirmDisconnectOdoo(t *testing.T) {
	tests := []struct {
		name             string
		dialogResult     string
		dialogErr        error
		expectDisconnect bool
		expectErr        bool
	}{
		{name: "confirm disconnect", dialogResult: "Disconnect", expectDisconnect: true},
		{name: "cancel disconnect", dialogResult: "Cancel", expectDisconnect: false},
		{name: "dialog error", dialogErr: errors.New("dialog failed"), expectErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cfg, err := config.NewManager()
			testutil.ExpectedNoError(t, err)
			_ = cfg.SetOdooCredentials("http://127.0.0.1:8069", "tok", "uuid-1")

			app := createTestApp(t, cfg)
			dialogs := &fakeDialogs{messageResult: tc.dialogResult, messageErr: tc.dialogErr}
			events := &fakeEvents{}
			app.dialogs = dialogs
			app.events = events

			disconnected, err := app.ConfirmDisconnectOdoo()
			if tc.expectErr {
				testutil.ExpectedError(t, err)
			} else {
				testutil.ExpectedNoError(t, err)
			}
			testutil.ExpectedEqual(t, disconnected, tc.expectDisconnect)

			if tc.expectDisconnect {
				testutil.ExpectedFalse(t, cfg.HasOdooCredentials())
				testutil.ExpectedLen(t, events.emitted, 1)
				testutil.ExpectedEqual(t, events.emitted[0].Name, "odoo:status_changed")
			} else {
				testutil.ExpectedTrue(t, cfg.HasOdooCredentials())
				testutil.ExpectedLen(t, events.emitted, 0)
			}
		})
	}
}

func TestApp_AppVariableAndPrintersAndGetPrinterIp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedNoError(t, cfg.AddLanEposPrinter("192.168.1.100"))

	app := createTestApp(t, cfg)
	appVariable := app.AppVariable()
	testutil.ExpectedEqual(t, app.GetPrinterIp("czpTTjEyMzQ1Ng"), fmt.Sprintf("127.0.0.1:%d/p/czpTTjEyMzQ1Ng", cfg.Data.Port))
	testutil.ExpectedTrue(t, appVariable.ServerRunning)
	testutil.ExpectedTrue(t, appVariable.Os != "")

	printers := app.Printers()
	foundLAN := false
	for _, p := range printers.Printers {
		if p.IsLAN && p.LANIp == "192.168.1.100" {
			foundLAN = true
			testutil.ExpectedEqual(t, p.Type, string(printer.TypeReceipt))
			testutil.ExpectedEqual(t, p.Name, "Network - 192.168.1.100")
			testutil.ExpectedEqual(t, p.Ip, fmt.Sprintf("%s/p/%s", app.webserver.LocalAddr(), p.Identifier))
		}
	}
	testutil.ExpectedTrue(t, foundLAN)
}

func TestApp_AddLANPrinter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)

	app := &App{config: cfg, printerManager: printer.NewManager()}

	// Invalid format
	testutil.ExpectedError(t, app.AddLANPrinter("not.an.ip"))
	testutil.ExpectedError(t, app.AddLANPrinter("  "))

	// Unreachable
	testutil.ExpectedError(t, app.AddLANPrinter("127.0.0.254"))

	// Reachable
	_, _, err = testutil.StartMockTCPServer(t)
	testutil.ExpectedNoError(t, err)

	testutil.ExpectedNoError(t, app.AddLANPrinter("127.0.0.1"))
	printers := cfg.GetLANPrinters()
	testutil.ExpectedLen(t, printers, 1)
	testutil.ExpectedEqual(t, printers[0], "127.0.0.1")
}

func TestApp_CheckLANPrinterStatus(t *testing.T) {
	app := &App{printerManager: printer.NewManager()}

	testutil.ExpectedFalse(t, app.CheckLANPrinterStatus("127.0.0.254"))

	_, _, err := testutil.StartMockTCPServer(t)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedTrue(t, app.CheckLANPrinterStatus("127.0.0.1"))
}

func TestApp_ConfirmRemoveLANPrinter(t *testing.T) {
	const ip = "192.168.1.100"

	tests := []struct {
		name          string
		dialogResult  string
		dialogErr     error
		expectRemoved bool
		expectErr     bool
	}{
		{name: "confirm removes printer", dialogResult: "Confirm", expectRemoved: true},
		{name: "linux yes button removes printer", dialogResult: "Yes", expectRemoved: true},
		{name: "cancel keeps printer", dialogResult: "Cancel"},
		{name: "dialog error keeps printer", dialogErr: errors.New("no display"), expectErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cfg, err := config.NewManager()
			testutil.ExpectedNoError(t, err)
			testutil.ExpectedNoError(t, cfg.AddLanEposPrinter(ip))

			dialogs := &fakeDialogs{messageResult: tc.dialogResult, messageErr: tc.dialogErr}
			app := &App{config: cfg, printerManager: printer.NewManager(), dialogs: dialogs}

			removed, err := app.ConfirmRemoveLANPrinter(ip)
			if tc.expectErr {
				testutil.ExpectedError(t, err)
			} else {
				testutil.ExpectedNoError(t, err)
			}
			testutil.ExpectedEqual(t, removed, tc.expectRemoved)

			expectedRemaining := 1
			if tc.expectRemoved {
				expectedRemaining = 0
			}
			testutil.ExpectedLen(t, cfg.GetLANPrinters(), expectedRemaining)
			testutil.ExpectedLen(t, dialogs.messages, 1)
			testutil.ExpectedContains(t, dialogs.messages[0].Message, ip)
		})
	}
}

func TestApp_ConfirmQuit(t *testing.T) {
	tests := []struct {
		name         string
		dialogResult string
		dialogErr    error
		expectQuit   bool
	}{
		{name: "quit button confirms", dialogResult: "Quit", expectQuit: true},
		{name: "linux yes button confirms", dialogResult: "Yes", expectQuit: true},
		{name: "cancel does not quit", dialogResult: "Cancel"},
		{name: "dialog error does not quit", dialogErr: errors.New("no display")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := &App{dialogs: &fakeDialogs{messageResult: tc.dialogResult, messageErr: tc.dialogErr}}
			testutil.ExpectedEqual(t, app.ConfirmQuit(), tc.expectQuit)
		})
	}
}

func TestApp_DownloadLogs(t *testing.T) {
	initLogs := func(t *testing.T) string {
		t.Helper()
		t.Setenv("HOME", t.TempDir())
		logger.InitLogger()
		dir := logger.LogDirectory()
		testutil.ExpectedTrue(t, dir != "")
		return dir
	}

	t.Run("writes archive to the chosen path", func(t *testing.T) {
		initLogs(t)
		savePath := filepath.Join(t.TempDir(), "logs.zip")

		dialogs := &fakeDialogs{savePath: savePath}
		app := &App{dialogs: dialogs}

		app.DownloadLogs()

		info, err := os.Stat(savePath)
		testutil.ExpectedNoError(t, err)
		testutil.ExpectedTrue(t, info.Size() > 0)
		testutil.ExpectedLen(t, dialogs.saves, 1)
		testutil.ExpectedContains(t, dialogs.saves[0].DefaultFilename, "epos-proxy-logs-")
	})

	t.Run("cancelling the save dialog writes nothing", func(t *testing.T) {
		initLogs(t)
		dialogs := &fakeDialogs{savePath: ""}
		app := &App{dialogs: dialogs}
		app.DownloadLogs()
		testutil.ExpectedLen(t, dialogs.messages, 0)
	})

	t.Run("save dialog error is reported", func(t *testing.T) {
		initLogs(t)
		dialogs := &fakeDialogs{saveErr: errors.New("dialog unavailable")}
		app := &App{dialogs: dialogs}
		app.DownloadLogs()
		testutil.ExpectedLen(t, dialogs.messages, 1)
		testutil.ExpectedEqual(t, dialogs.messages[0].Type, wailsruntime.ErrorDialog)
	})

	t.Run("zip failure is reported", func(t *testing.T) {
		initLogs(t)
		savePath := filepath.Join(t.TempDir(), "missing", "logs.zip")
		dialogs := &fakeDialogs{savePath: savePath}
		app := &App{dialogs: dialogs}
		app.DownloadLogs()
		testutil.ExpectedLen(t, dialogs.messages, 1)
		testutil.ExpectedEqual(t, dialogs.messages[0].Type, wailsruntime.ErrorDialog)
	})
}

func TestApp_AutostartMethods(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	app := NewApp()
	testutil.ExpectedNoError(t, app.EnableAutostart())
	_ = app.DisableAutostart()
}
