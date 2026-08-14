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
	testutil.ExpectedNotNil(t, app.printerManager)
}

func TestApp_AppVariableAndPrintersAndGetPrinterIp(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)

	err = cfg.AddLanEposPrinter("192.168.1.100")
	testutil.ExpectedNoError(t, err)

	port := testutil.GetFreePort(t)
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

	// Verify Printers() includes the configured LAN printer
	printers := app.Printers()
	foundLAN := false
	for _, p := range printers.Printers {
		if p.IsLAN && p.LANIp == "192.168.1.100" {
			foundLAN = true
			testutil.ExpectedEqual(t, p.Type, string(printer.TypeReceipt))
			testutil.ExpectedEqual(t, p.Name, "Network - 192.168.1.100")
			testutil.ExpectedEqual(t, p.Ip, fmt.Sprintf("127.0.0.1:%d/p/%s", port, p.Id))
		}
	}
	testutil.ExpectedTrue(t, foundLAN, "Expected to find configured LAN printer in printer status")
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
	err = app.AddLANPrinter("127.0.0.254")
	testutil.ExpectedError(t, err)

	// Reachable printer.
	_, _, err = testutil.StartMockTCPServer(t)
	testutil.ExpectedNoError(t, err)

	err = app.AddLANPrinter("127.0.0.1")
	testutil.ExpectedNoError(t, err)

	printers := cfg.GetLANPrinters()
	testutil.ExpectedLen(t, printers, 1)
	testutil.ExpectedEqual(t, printers[0], "127.0.0.1")
}

func TestApp_CheckLANPrinterStatus(t *testing.T) {
	app := &App{}

	// 1. Unreachable (closed IP returns false)
	testutil.ExpectedFalse(t, app.CheckLANPrinterStatus("127.0.0.254"))

	// 2. Active listener using StartMockTCPServer
	_, _, err := testutil.StartMockTCPServer(t)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedTrue(t, app.CheckLANPrinterStatus("127.0.0.1"))
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
