package server

import (
	"epos-proxy/internal/logger"
	"github.com/gofiber/fiber/v3"
)

func registerOboxRoutes(s *Server) {
	if s.obox == nil {
		return
	}

	s.app.Get("/odoo/", s.obox.HandleDiscovery)
	s.app.Get("/odoo/connect", s.obox.HandleConnect)
	s.app.Get("/odoo/disconnect", s.obox.HandleDisconnect)
	s.app.Get("/odoo/health", s.obox.HandleHealth)
	s.app.Get("/odoo/discover_devices", s.obox.HandleDiscoverDevices)

	s.app.Post("/usb/v1/printer/:printerId/cgi-bin/epos/service.cgi", func(ctx fiber.Ctx) error {
		printerId := ctx.Params("printerId")
		logger.Debugf("Obox ePOS print request received for printer: %s", printerId)
		return printReceipt(s.mgr, ctx, printerId)
	})

	s.app.Post("/usb/v1/printer/:printerId/pstprnt", func(ctx fiber.Ctx) error {
		printerId := ctx.Params("printerId")
		logger.Debugf("Obox label print request received for printer: %s", printerId)
		return printLabel(s.mgr, ctx, printerId)
	})

	notSupported := func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "service not supported on Obox app",
		})
	}

	s.app.Get("/odoo/restart", notSupported)
	s.app.Get("/sos/v1/enable", notSupported)
	s.app.Get("/sos/v1/disable", notSupported)
	s.app.Post("/display/v1/update-url", notSupported)
	s.app.Get("/wifi/status", notSupported)
	s.app.Get("/wifi/networks", notSupported)
	s.app.Post("/leds/set", notSupported)
	s.app.Post("/usb/v1/printer/print", notSupported)
}
