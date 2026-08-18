package obox

import (
	"epos-proxy/internal/logger"

	"github.com/gofiber/fiber/v3"
)

func (m *Module) handleDiscovery(ctx fiber.Ctx) error {
	logger.Debug("[obox] LAN health check /odoo/")
	dbURL, _, _ := m.GetCredentials()
	if dbURL != "" {
		serial := m.appID
		if serial == "" && m.cfg != nil {
			serial = m.cfg.GetAppID()
		}
		return ctx.JSON(map[string]interface{}{
			"status": "configured",
			"data": map[string]string{
				"serial": serial,
				"db_url": dbURL,
			},
		})
	}
	return ctx.JSON(map[string]interface{}{
		"status": "not_configured",
	})
}

func (m *Module) handleHealth(ctx fiber.Ctx) error {
	logger.Infof("[obox] /odoo/health ping")
	if m.IsConnected() {
		go m.callOdooPing()
	}
	return ctx.JSON(map[string]string{"status": "ok"})
}

func (m *Module) handleRestart(ctx fiber.Ctx) error {
	logger.Infof("[obox] restart — ignored, returning success")
	return ctx.JSON(map[string]string{"status": "restarted"})
}

func (m *Module) handleDisconnect(ctx fiber.Ctx) error {
	logger.Infof("[obox] disconnect — clearing device credentials")
	m.Disconnect()
	return ctx.JSON(map[string]string{"status": "disconnected"})
}

func (m *Module) handleDiscoverDevices(ctx fiber.Ctx) error {
	logger.Infof("[obox] discover_devices")
	devices := m.buildDeviceList()
	return ctx.JSON(devices)
}

func (m *Module) handleConnect(ctx fiber.Ctx) error {
	dbURL := ctx.Query("db_url")
	token := ctx.Query("token")
	dbUUID := ctx.Query("db_uuid")

	logger.Infof("[obox] offline connect received: db_url=%s, token=%s, db_uuid=%s", dbURL, token, dbUUID)
	if dbURL != "" && token != "" {
		m.SetCredentials(dbURL, token, dbUUID)
		m.setLiveStatus("connecting")
		if m.cfg != nil {
			if err := m.cfg.SetOdooCredentials(dbURL, token, dbUUID); err != nil {
				logger.Warnf("[obox] Failed to save Odoo credentials to storage: %v", err)
			}
		}
		go m.callOdooOboxConnect(dbURL, token, dbUUID)
	}
	return ctx.SendStatus(fiber.StatusOK)
}
