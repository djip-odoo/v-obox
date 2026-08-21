package server

import (
	"fmt"
	"sync/atomic"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"
	"epos-proxy/internal/obox"
	"epos-proxy/internal/printer"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

type Server struct {
	app     *fiber.App
	Port    int
	running atomic.Bool
	cfg     *config.Manager
	obox    *obox.Module
	mgr     *printer.Manager
}

func New(port int, mgr *printer.Manager, cfg *config.Manager) *Server {
	app := fiber.New(fiber.Config{AppName: "Obox app"})
	app.Use(cors.New(cors.Config{
		AllowOrigins:        []string{"*"},
		AllowPrivateNetwork: true,
	}))

	server := &Server{
		app:  app,
		Port: port,
		cfg:  cfg,
		mgr:  mgr,
	}

	server.obox = obox.Manager(cfg, mgr, server.LocalAddr)
	server.registerRoutes()
	server.running.Store(true)
	go func() {
		logger.Infof("HTTP server listening on 0.0.0.0:%d", port)
		err := app.Listen(fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			logger.Error("EPOS Server Error: ", err)
		}
		server.running.Store(false)
		logger.Warn("HTTP server stopped")
	}()
	return server
}

func (s *Server) Stop() error {
	logger.Infof("Stopping HTTP server")
	s.running.Store(false)
	return s.app.Shutdown()
}

func (s *Server) Running() bool {
	return s.running.Load()
}

func (s *Server) AppID() string {
	return s.cfg.GetAppID()
}

func (s *Server) LocalAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", s.Port)
}

func (s *Server) GetWebsocketStatus() string {
	if s.obox != nil {
		return s.obox.GetWebsocketStatus()
	}
	return "disconnected"
}

func (s *Server) GetOdooDbURL() string {
	if s.obox != nil {
		if url := s.obox.GetDbURL(); url != "" {
			return url
		}
	}
	return s.cfg.GetOdooDbURL()
}

func (s *Server) DisconnectOdoo() {
	if s.obox != nil {
		s.obox.Disconnect()
	}
}

func (s *Server) OnStatusChange(listener obox.StatusListener) {
	if s.obox != nil {
		s.obox.OnStatusChange(listener)
	}
}

func (s *Server) registerRoutes() {
	registerEPOSRoutes(s)
	registerOboxRoutes(s)
}
