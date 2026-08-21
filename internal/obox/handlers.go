package obox

import (
	"epos-proxy/internal/logger"

	"github.com/gofiber/fiber/v3"
)

func (m *Module) HandleDiscovery(ctx fiber.Ctx) error {
	logger.Debug("[obox] LAN health check /odoo/")
	m.RecordLANContact()
	if dbURL, _ := m.GetCredentials(); dbURL != "" {
		return ctx.JSON(map[string]interface{}{
			"status": "configured",
			"data": map[string]string{
				"serial": m.appID,
				"db_url": dbURL,
			},
		})
	}
	return ctx.JSON(map[string]interface{}{"status": "not_configured", "data": nil})
}

func (m *Module) HandleConnect(ctx fiber.Ctx) error {
	dbURL := ctx.Query("db_url")
	token := ctx.Query("token")
	dbUUID := ctx.Query("db_uuid")

	logger.Infof("[obox] offline connect received: db_url=%s, db_uuid=%s", dbURL, dbUUID)
	if dbURL == "" || token == "" {
		return ctx.Status(fiber.StatusBadRequest).JSON(map[string]interface{}{
			"status":  "error",
			"message": "missing required parameters: db_url and token",
		})
	}

	m.RecordLANContact()
	m.SetCredentials(dbURL, token, dbUUID)
	m.setLiveStatus("connecting")
	go m.callOdooOboxConnect(dbURL, token, dbUUID)

	return ctx.JSON(map[string]interface{}{
		"status":  "success",
		"message": "Connecting to Odoo database",
	})
}

func (m *Module) HandleDisconnect(ctx fiber.Ctx) error {
	logger.Infof("[obox] /odoo/disconnect request received")
	m.Disconnect()
	return ctx.JSON(map[string]interface{}{"status": "disconnected"})
}

func (m *Module) HandleHealth(ctx fiber.Ctx) error {
	logger.Debug("[obox] /odoo/health request received")
	go m.callOdooPing()
	return ctx.JSON(map[string]string{"status": "ok"})
}

func (m *Module) HandleDiscoverDevices(ctx fiber.Ctx) error {
	logger.Debug("[obox] /odoo/discover_devices request received")
	devices := m.buildDeviceList()
	return ctx.JSON(devices)
}
