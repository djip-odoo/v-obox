package obox

import (
	"epos-proxy/internal/logger"

	"github.com/gofiber/fiber/v3"
)

func (m *Module) HandleDiscovery(ctx fiber.Ctx) error {
	logger.Debug("[obox] LAN health check /odoo/")
	if dbURL, _ := m.GetCredentials(); dbURL != "" {
		return ctx.JSON(map[string]interface{}{
			"status": "configured",
			"data": map[string]string{
				"serial": m.appID,
				"db_url": dbURL,
			},
		})
	}
	return ctx.JSON(map[string]interface{}{"status": "not_configured"})
}

func (m *Module) HandleConnect(ctx fiber.Ctx) error {
	dbURL := ctx.Query("db_url")
	token := ctx.Query("token")
	dbUUID := ctx.Query("db_uuid")

	logger.Infof("[obox] offline connect received: db_url=%s, token=%s, db_uuid=%s", dbURL, token, dbUUID)
	if dbURL == "" || token == "" {
		return ctx.Status(fiber.StatusBadRequest).JSON(map[string]interface{}{
			"error": "missing required parameters: db_url and token",
		})
	}

	m.SetCredentials(dbURL, token, dbUUID)
	m.setLiveStatus("connecting")
	if err := m.cfg.SetOdooCredentials(dbURL, token, dbUUID); err != nil {
		logger.Warnf("[obox] Failed to save Odoo credentials to storage: %v", err)
	}
	go m.callOdooOboxConnect(dbURL, token, dbUUID)

	return ctx.SendStatus(fiber.StatusOK)
}

func (m *Module) HandleDisconnect(ctx fiber.Ctx) error {
	logger.Infof("[obox] /odoo/disconnect request received")
	m.Disconnect()
	return ctx.JSON(map[string]interface{}{"status": "disconnected"})
}
