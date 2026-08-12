package server

import (
	"fmt"
	"net"
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

// GetLocalIP returns the outbound IPv4 address of the machine, or 127.0.0.1 if none is found.
func GetLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
		if ok && localAddr.IP != nil && !localAddr.IP.IsLoopback() {
			if ip4 := localAddr.IP.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && !ip.IsLoopback() {
					if ip4 := ip.To4(); ip4 != nil {
						return ip4.String()
					}
				}
			}
		}
	}

	return "127.0.0.1"
}

// LocalIP returns the local IP address of the machine.
func (s *Server) LocalIP() string {
	return GetLocalIP()
}

// LocalAddr returns the host:port address of the server matching its current IP and running port.
func (s *Server) LocalAddr() string {
	return fmt.Sprintf("%s:%d", s.LocalIP(), s.Port)
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
