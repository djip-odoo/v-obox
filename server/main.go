package server

import (
	"fmt"
	"sync"
	"sync/atomic"

	"epos-proxy/config"
	"epos-proxy/logger"
	"epos-proxy/printer"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

// Server represents the HTTP proxy server.
type Server struct {
	app     *fiber.App
	Port    int
	running atomic.Bool

	// mgr is kept so device modules / printers can list real printers
	mgr *printer.Manager
	// cfg is kept for LAN printer listing
	cfg *config.Manager
}

// Binder is a function that configures routes, middlewares, or background workers on a Server instance.
type Binder func(s *Server)

var (
	bindersMu sync.Mutex
	binders   []Binder
)

// Register registers a binder function to be invoked automatically whenever a Server is initialized.
// Any file in package server (e.g. obox.go, epos.go, scale.go) can call Register in its init() function
// to automatically bind its routes and services without modifying main.go.
func Register(b Binder) {
	bindersMu.Lock()
	defer bindersMu.Unlock()
	binders = append(binders, b)
}

// App returns the underlying Fiber app instance.
func (s *Server) App() *fiber.App {
	return s.app
}

// Mgr returns the printer manager instance.
func (s *Server) Mgr() *printer.Manager {
	return s.mgr
}

// Cfg returns the configuration manager instance.
func (s *Server) Cfg() *config.Manager {
	return s.cfg
}

// ── Constructor ────────────────────────────────────────────────────────────────

func New(port int, mgr *printer.Manager, cfg ...*config.Manager) *Server {
	app := fiber.New(fiber.Config{
		AppName: "ePOS proxy",
	})
	app.Use(cors.New(cors.Config{
		AllowOrigins:        []string{"*"},
		AllowPrivateNetwork: true,
	}))

	var conf *config.Manager
	if len(cfg) > 0 {
		conf = cfg[0]
	}

	s := &Server{
		app:  app,
		Port: port,
		mgr:  mgr,
		cfg:  conf,
	}

	// Automatically run all registered module binders
	bindersMu.Lock()
	registered := make([]Binder, len(binders))
	copy(registered, binders)
	bindersMu.Unlock()

	for _, b := range registered {
		b(s)
	}

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
