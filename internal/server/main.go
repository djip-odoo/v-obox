package server

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
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
	ln      net.Listener
	Port    int
	mgr     *printer.Manager
	running atomic.Bool
	cfg     *config.Manager
	obox    *obox.Module
}

func New(cfg *config.Manager) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("unable to start server: config manager is required")
	}

	port, err := cfg.ResolvePort()
	if err != nil {
		return nil, fmt.Errorf("unable to start server: %w", err)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return nil, fmt.Errorf("unable to start server: %w", err)
	}

	app := fiber.New(fiber.Config{AppName: "Obox app"})
	app.Use(cors.New(cors.Config{
		AllowOrigins:        []string{"*"},
		AllowPrivateNetwork: true,
	}))

	s := &Server{
		app:  app,
		ln:   ln,
		Port: port,
		mgr:  printer.NewManager(),
		cfg:  cfg,
	}

	s.obox = obox.Manager(cfg, s.mgr, s.LocalAddr)
	s.registerRoutes(cfg)

	s.running.Store(true)
	go func() {
		logger.Infof("HTTP server listening on 0.0.0.0:%d", port)
		if err := app.Listener(ln); err != nil {
			logger.Error("EPOS Server Error: ", err)
		}
		s.running.Store(false)
		logger.Warn("HTTP server stopped")
	}()

	return s, nil
}

func (s *Server) Stop() error {
	logger.Infof("Stopping HTTP server")
	if !s.running.Swap(false) {
		return nil
	}
	if s.obox != nil {
		s.obox.Stop()
	}
	var closeErr error
	if s.ln != nil {
		closeErr = s.ln.Close()
		if closeErr != nil && (errors.Is(closeErr, net.ErrClosed) || strings.Contains(closeErr.Error(), "use of closed network connection")) {
			closeErr = nil
		}
	}
	err := s.app.Shutdown()
	if err != nil && (errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "use of closed network connection")) {
		err = nil
	}
	if err != nil {
		return err
	}
	return closeErr
}

func (s *Server) Running() bool {
	return s.running.Load()
}

func (s *Server) App() *fiber.App {
	return s.app
}

func (s *Server) AppID() string {
	if s.cfg != nil {
		return s.cfg.GetAppID()
	}
	return ""
}

func (s *Server) LocalAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", s.Port)
}

func (s *Server) GetPrinterIp(id string) string {
	ip := fmt.Sprintf("%s/p/%s", s.LocalAddr(), id)
	logger.Debugf("Generated printer endpoint: %s", ip)
	return ip
}

func (s *Server) Obox() *obox.Module {
	return s.obox
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
	if s.cfg != nil {
		return s.cfg.GetOdooDbURL()
	}
	return ""
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

// RouteBinder registers routes for a Server instance.
type RouteBinder func(s *Server, cfg *config.Manager)

var (
	routeBinders   []RouteBinder
	routeBindersMu sync.Mutex
)

func RegisterRoute(binder RouteBinder) {
	routeBindersMu.Lock()
	defer routeBindersMu.Unlock()
	routeBinders = append(routeBinders, binder)
}

func (s *Server) registerRoutes(cfg *config.Manager) {
	routeBindersMu.Lock()
	binders := make([]RouteBinder, len(routeBinders))
	copy(binders, routeBinders)
	routeBindersMu.Unlock()

	for _, binder := range binders {
		binder(s, cfg)
	}
}
