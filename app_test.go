package main

import (
	"fmt"
	"testing"

	"epos-proxy/internal/config"
	"epos-proxy/internal/printer"
	"epos-proxy/internal/server"
	"epos-proxy/internal/testutil"
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

	port := testutil.GetFreePort(t)

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
	testutil.ExpectedEqual(t, app.GetPrinterIp("czpTTjEyMzQ1Ng"), fmt.Sprintf("127.0.0.1:%d/p/czpTTjEyMzQ1Ng", port))
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
	if err := printer.CheckLANPrinter("127.0.0.1"); err != nil {
		_, _, mockErr := testutil.StartMockTCPServer(t, printer.LANPort)
		if mockErr != nil {
			t.Skipf("Cannot bind port %d for live LAN add test: %v", printer.LANPort, mockErr)
		}
	}

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
