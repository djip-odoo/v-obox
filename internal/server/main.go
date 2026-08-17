package server

import (
	"fmt"
	"sync"
	"sync/atomic"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

type Server struct {
	app     *fiber.App
	Port    int
	running atomic.Bool

	obox *oboxModule

	statusListenersMu sync.RWMutex
	statusListeners   []StatusListener
}

func (s *Server) App() *fiber.App {
	return s.app
}

func (s *Server) LocalAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", s.Port)
}

func New(port int, mgr *printer.Manager, cfg *config.Manager) *Server {
	app := fiber.New(fiber.Config{
		AppName: "ePOS proxy",
	})
	app.Use(cors.New(cors.Config{
		AllowOrigins:        []string{"*"},
		AllowPrivateNetwork: true,
	}))

	s := &Server{app: app, Port: port}

	registerEPOSRoutes(app, mgr)
	registerOboxRoutes(s, mgr, cfg)

	s.running.Store(true)
	go func() {
		logger.Infof("HTTP server listening on 0.0.0.0:%d", port)
		err := app.Listen(fmt.Sprintf("0.0.0.0:%d", port))
		if err != nil {
			logger.Error("EPOS Server Error: ", err)
		}
		s.running.Store(false)
		logger.Warn("HTTP server stopped")
	}()

	return s
}

func (s *Server) Stop() error {
	logger.Infof("Stopping HTTP server")
	return s.app.Shutdown()
}

func (s *Server) Running() bool {
	return s.running.Load()
}
