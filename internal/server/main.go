package server

import (
	"fmt"
	"sync"
	"sync/atomic"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"
	"epos-proxy/internal/util"

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

	obox *oboxModule
}

// OdooStatus represents the connection and websocket status between proxy and Odoo.
type OdooStatus struct {
	AppId           string `json:"appId"`
	IpAddress       string `json:"ipAddress"`
	Connected       bool   `json:"connected"`
	DbURL           string `json:"dbUrl"`
	WebsocketStatus string `json:"websocketStatus"`
	Serial          string `json:"serial"`
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

// LocalAddr returns the host:port address of the server matching its current IP and running port.
func (s *Server) LocalAddr() string {
	return fmt.Sprintf("%s:%d", util.GetLocalIP(false), s.Port)
}

// AppID returns the application ID configured or generated on this server.
func (s *Server) AppID() string {
	if s.cfg != nil {
		return s.cfg.GetAppID()
	}
	return ""
}

// GetOdooStatus returns the current connection and websocket status with Odoo.
func (s *Server) GetOdooStatus() OdooStatus {
	if s.obox != nil {
		return s.obox.getStatus()
	}
	if s.cfg != nil && s.cfg.HasOdooCredentials() {
		return OdooStatus{
			AppId:           s.AppID(),
			IpAddress:       s.LocalAddr(),
			Connected:       true,
			DbURL:           s.cfg.GetOdooDbURL(),
			WebsocketStatus: "connected",
			Serial:          s.cfg.GetOdooSerial(),
		}
	}
	return OdooStatus{
		AppId:           s.AppID(),
		IpAddress:       s.LocalAddr(),
		Connected:       false,
		WebsocketStatus: "disconnected",
		Serial:          s.AppID(),
	}
}

// DisconnectOdoo clears in-memory device credentials and removes stored config.
func (s *Server) DisconnectOdoo() {
	if s.obox != nil {
		s.obox.disconnect()
	}
	if s.cfg != nil {
		_ = s.cfg.ClearOdooConfig()
	}
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
	if len(cfg) > 0 && cfg[0] != nil {
		conf = cfg[0]
	} else {
		conf = config.NewManagerWithPath("")
	}
	_, _ = conf.EnsureAppID()

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
