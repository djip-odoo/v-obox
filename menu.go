package main

import (
	"runtime"

	"epos-proxy/internal/logger"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func createMenu(app *application.App, appService *App) *application.Menu {
	mainMenu := app.NewMenu()
	appMenu := mainMenu.AddSubmenu("App")

	if runtime.GOOS == "darwin" {
		// Without an Edit menu, copy/paste do nothing on the webview on macOS
		mainMenu.AddRole(application.EditMenu)
	}

	appMenu.AddCheckbox("Auto Start", appService.IsAutostartEnabled()).OnClick(func(ctx *application.Context) {
		handleAutoStartToggle(appService, ctx)
	})

	appMenu.AddCheckbox("Allow Network Printing", appService.IsNetworkPrintingEnabled()).OnClick(func(ctx *application.Context) {
		handleNetworkPrintingToggle(appService, ctx)
	})

	appMenu.Add("Set PIN").OnClick(func(_ *application.Context) {
		appService.EmitEvent("open-set-pin-dialog")
	})

	appMenu.Add("Download Logs").OnClick(func(_ *application.Context) {
		appService.DownloadLogs()
	})

	appMenu.Add("Open Web App").OnClick(func(_ *application.Context) {
		url := appService.config.GetWebViewURL()
		if url != "" {
			appService.NavigateToWebapp(url)
		}
	})

	appMenu.Add("Switch to App UI").OnClick(func(_ *application.Context) {
		appService.NavigateToLocalUI()
	})

	appMenu.Add("Return to Proxy Settings").SetAccelerator("Escape").OnClick(func(_ *application.Context) {
		logger.Infof("Return to Proxy Settings requested from menu")
		appService.NavigateToLocalUI()
	})

	appMenu.AddSeparator()

	appMenu.Add("Quit").OnClick(func(_ *application.Context) {
		if appService.ConfirmQuit() {
			logger.Infof("Quit requested by user")
			app.Quit()
		}
	})

	return mainMenu
}

func handleAutoStartToggle(appService *App, ctx *application.Context) {
	menuItem := ctx.ClickedMenuItem()
	checked := menuItem.Checked()

	logger.Debugf("Auto Start toggled: %v", checked)

	if checked {
		if err := appService.EnableAutostart(); err != nil {
			logger.Errorf("Failed to enable autostart: %v", err)
		}
		return
	}

	if err := appService.DisableAutostart(); err != nil {
		logger.Errorf("Failed to disable autostart: %v", err)
	}
}

func handleNetworkPrintingToggle(appService *App, ctx *application.Context) {
	menuItem := ctx.ClickedMenuItem()
	checked := menuItem.Checked()

	// Guards against a spurious callback firing with the already-persisted value
	if checked == appService.IsNetworkPrintingEnabled() {
		return
	}

	logger.Debugf("Allow Network Printing toggled: %v", checked)

	if err := appService.SetNetworkPrintingEnabled(checked); err != nil {
		logger.Errorf("Failed to set network printing enabled: %v", err)
		return
	}

	appService.EmitEvent("network-printing-changed", checked)
}

func (a *App) ConfirmQuit() bool {
	return a.dlg().Question("Quit ePOS Proxy", "Stopping the proxy will prevent POS from printing receipts.\n\nAre you sure you want to quit?")
}
