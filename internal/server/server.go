package server

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"

	"epos-proxy/internal/config"
	"epos-proxy/internal/escpos"
	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/google/uuid"
)

// EPOSResponse is the XML envelope returned by print proxy routes.
type EPOSResponse struct {
	XMLName xml.Name `xml:"response"`
	Success bool     `xml:"success,attr"`
	Code    string   `xml:"code,attr"`
	Status  string   `xml:"status,attr"`
}

// Server wraps the Fiber HTTP server with auth state.
type Server struct {
	app             *fiber.App
	Port            int
	running         atomic.Bool
	cfg             *config.Manager
	mgr             *printer.Manager
	mu              sync.RWMutex
	wailsToken      string          // trusted Wails session token (empty = none set yet)
	sessions        map[string]bool // active PIN-auth session tokens (remote clients)
	onKioskChanged  func(enabled bool)
	onConfigChanged func()
	onKioskReload   func()
}

// SetKioskCallback registers a callback invoked when kiosk enabled status changes via HTTP API.
func (s *Server) SetKioskCallback(cb func(bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onKioskChanged = cb
}

// SetConfigCallback registers a callback invoked when webview configuration changes via HTTP API.
func (s *Server) SetConfigCallback(cb func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onConfigChanged = cb
}

// SetKioskReloadCallback registers a callback invoked when kiosk reload is requested via HTTP API.
func (s *Server) SetKioskReloadCallback(cb func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onKioskReload = cb
}

// SetSessionToken registers the trusted Wails session token.
// Called once from App.startup() after the token is generated.
func (s *Server) SetSessionToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wailsToken = token
}

// CreatePINSession validates pin against the stored PIN. On success it issues
// an opaque session token the remote client can use for privileged calls.
// Returns ("", false) when pin is wrong or no PIN is configured.
func (s *Server) CreatePINSession(pin string) (string, bool) {
	if s.cfg == nil || !s.cfg.CheckWebViewPIN(pin) {
		return "", false
	}
	token := uuid.New().String()
	s.mu.Lock()
	s.sessions[token] = true
	s.mu.Unlock()
	return token, true
}

// isAuthenticated returns true when c carries either the Wails token or a
// valid PIN-session token.
func (s *Server) isAuthenticated(c fiber.Ctx) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Trusted Wails request
	if wt := c.Get("X-Wails-Token"); wt != "" && s.wailsToken != "" && wt == s.wailsToken {
		return true
	}

	// Remote PIN-session
	auth := c.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return s.sessions[strings.TrimPrefix(auth, "Bearer ")]
	}
	return false
}

// requireAuth is a Fiber middleware that blocks unauthenticated remote requests.
func (s *Server) requireAuth(c fiber.Ctx) error {
	if s.isAuthenticated(c) {
		return c.Next()
	}
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "authentication required"})
}

// New creates and starts the HTTP server.
//
// mgr   — printer manager (required)
// cfg   — config manager; may be nil in tests that don't exercise APIs
// distFS — embedded frontend/dist; may be nil (falls back to hello-world at /)
func New(port int, mgr *printer.Manager, cfg *config.Manager, distFS fs.FS) *Server {
	srv := &Server{
		Port:     port,
		cfg:      cfg,
		mgr:      mgr,
		sessions: make(map[string]bool),
	}

	app := fiber.New(fiber.Config{
		AppName: "ePOS proxy",
	})
	srv.app = app

	app.Use(cors.New(cors.Config{
		AllowOrigins:        []string{"*"},
		AllowMethods:        []string{"GET", "POST", "HEAD", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders:        []string{"*"},
		AllowPrivateNetwork: true,
	}))

	// Ensure Access-Control-Allow-Private-Network is guaranteed on every response
	app.Use(func(c fiber.Ctx) error {
		c.Set("Access-Control-Allow-Private-Network", "true")
		if c.Method() == "OPTIONS" {
			c.Set("Access-Control-Allow-Origin", "*")
			c.Set("Access-Control-Allow-Methods", "GET, POST, HEAD, PUT, DELETE, PATCH, OPTIONS")
			c.Set("Access-Control-Allow-Headers", "*")
			return c.SendStatus(fiber.StatusNoContent)
		}
		return c.Next()
	})

	// ── Read-only APIs ────────────────────────────────────────────────────────

	app.Get("/api/app", srv.handleGetApp)
	app.Get("/api/printers", srv.handleGetPrinters)
	app.Get("/api/printers/lan/:ip/status", srv.handleGetLANPrinterStatus)
	app.Get("/api/webview", srv.handleGetWebView)
	app.Get("/api/troubleshoot", srv.handleGetTroubleshoot)

	// PIN session creation — validates PIN and issues a session token
	app.Post("/api/auth/session", srv.handleAuthSession)

	// ── Privileged APIs (require Wails token or PIN session) ─────────────────

	app.Post("/api/printers/lan", srv.requireAuth, srv.handleAddLANPrinter)
	app.Delete("/api/printers/lan", srv.requireAuth, srv.handleRemoveLANPrinter)
	app.Post("/api/webview/url", srv.requireAuth, srv.handleSetWebViewURL)
	app.Post("/api/webview/zoom", srv.requireAuth, srv.handleSetWebViewZoom)
	app.Post("/api/webview/enabled", srv.requireAuth, srv.handleSetWebViewEnabled)
	app.Post("/api/webview/reload", srv.requireAuth, srv.handleReloadWebView)

	// Test-print / cash-drawer via proxy — privileged so random remote callers
	// can't trigger prints, while Odoo POS continues to use the open /p/…
	// routes below.
	app.Post("/api/printers/:printerId/test-print", srv.requireAuth, srv.handleTestPrint)
	app.Post("/api/printers/:printerId/cash-drawer", srv.requireAuth, srv.handleCashDrawer)

	// ── ePOS print-proxy routes (open — also used by Odoo POS) ───────────────

	app.Post("/p/:printerId/cgi-bin/epos/service.cgi", func(ctx fiber.Ctx) error {
		printerId := ctx.Params("printerId")
		logger.Debugf("Print request received for printer: %s", printerId)
		return printData(mgr, ctx, printerId)
	})

	app.Post("/cgi-bin/epos/service.cgi", func(ctx fiber.Ctx) error {
		logger.Debugf("Print request received (auto printer selection)")
		return printData(mgr, ctx, "")
	})

	app.Post("/p/:printerId/pstprnt", func(ctx fiber.Ctx) error {
		printerId := ctx.Params("printerId")
		logger.Debugf("Label print request received for printer: %s", printerId)
		return printLabel(mgr, ctx, printerId)
	})

	// ── Embedded frontend (catch-all) ─────────────────────────────────────────

	if distFS != nil {
		app.Use("/", func(c fiber.Ctx) error {
			if c.Method() != "GET" {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
			}
			return serveFrontend(c, distFS)
		})
	} else {
		app.Get("/", func(ctx fiber.Ctx) error {
			return ctx.SendString(fmt.Sprintf("Hello from %s", app.Config().AppName))
		})
	}

	srv.running.Store(true)
	go func() {
		logger.Infof("HTTP server listening on 0.0.0.0:%d", port)
		if err := app.Listen(fmt.Sprintf("0.0.0.0:%d", port)); err != nil {
			logger.Error("EPOS Server Error: ", err)
		}
		srv.running.Store(false)
		logger.Warn("HTTP server stopped")
	}()
	return srv
}

// serveFrontend serves files from the embedded dist FS. Unrecognised paths
// fall back to index.html so the React SPA handles them.
func serveFrontend(c fiber.Ctx, distFS fs.FS) error {
	path := strings.TrimPrefix(c.Path(), "/")
	if path == "" {
		path = "index.html"
	}

	data, err := fs.ReadFile(distFS, path)
	if err != nil {
		// SPA fallback: unknown paths serve index.html
		data, err = fs.ReadFile(distFS, "index.html")
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		c.Type("html")
		return c.Send(data)
	}

	// Set Content-Type from extension
	ext := ""
	if dot := strings.LastIndex(path, "."); dot >= 0 {
		ext = path[dot:]
	}
	switch ext {
	case ".html":
		c.Type("html")
	case ".js", ".mjs":
		c.Set("Content-Type", "application/javascript; charset=utf-8")
	case ".css":
		c.Type("css")
	case ".ico":
		c.Type("ico")
	case ".png":
		c.Type("png")
	case ".svg":
		c.Set("Content-Type", "image/svg+xml")
	case ".json":
		c.Type("json")
	case ".woff":
		c.Set("Content-Type", "font/woff")
	case ".woff2":
		c.Set("Content-Type", "font/woff2")
	default:
		c.Set("Content-Type", "application/octet-stream")
	}

	return c.Send(data)
}

// ── ePOS print helpers (unchanged) ────────────────────────────────────────────

func printData(mgr *printer.Manager, ctx fiber.Ctx, printerID string) error {
	logger.Debugf("Processing print job for printer: %s", printerID)
	jobData, err := escpos.ParseXML(ctx.Body())
	if err != nil {
		logger.Errorf("XML parsing error: %v", err)
		return ctx.XML(EPOSResponse{Success: false, Code: "SchemaError", Status: ""})
	}
	logger.Debug("XML parsed successfully")

	reply, err := mgr.WriteAsync(printerID, jobData)
	if err == nil {
		logger.Debug("Print job queued")
		result := <-reply
		if !result.OK {
			err = result.Err
		}
	}
	if err != nil {
		retCode := ""
		if errors.Is(err, printer.ErrQueueFull) {
			retCode = "TooManyRequests"
			logger.Warn("Printer queue full")
		} else {
			retCode = "EX_BADPORT"
		}
		logger.Errorf("Print error [%s]: %v, Printer ID: %s", retCode, err, printerID)
		return ctx.XML(EPOSResponse{Success: false, Code: retCode, Status: ""})
	}
	logger.Debugf("Print job completed successfully for printer: %s", printerID)
	return ctx.XML(EPOSResponse{Success: true, Code: "", Status: ""})
}

func printLabel(mgr *printer.Manager, ctx fiber.Ctx, printerID string) error {
	jobData := ctx.Body()

	if len(jobData) == 0 {
		logger.Warn("Empty label data received")
		return ctx.SendStatus(fiber.StatusBadRequest)
	}

	logger.Debugf("Processing label print job for printer: %s", printerID)

	reply, err := mgr.WriteAsync(printerID, jobData)
	if err == nil {
		logger.Debug("Label print job queued")
		result := <-reply
		if !result.OK {
			err = result.Err
		}
	}

	if err != nil {
		if errors.Is(err, printer.ErrQueueFull) {
			logger.Warnf("Printer queue full, Printer ID: %s", printerID)
			return ctx.SendStatus(fiber.StatusTooManyRequests)
		}
		logger.Errorf("Print error: %v, Printer ID: %s", err, printerID)
		return ctx.SendStatus(fiber.StatusInternalServerError)
	}

	logger.Debugf("Print job completed successfully for printer: %s", printerID)
	return ctx.SendStatus(fiber.StatusOK)
}

// executePrint builds and dispatches an ePOS-XML print job through the
// printer manager. content is the inner ePOS XML (without the SOAP wrapper).
func (s *Server) executePrint(c fiber.Ctx, printerID, content string) error {
	body := fmt.Sprintf(
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>`+
			`<epos-print xmlns="http://www.epson-pos.com/schemas/2011/03/epos-print">`+
			`%s</epos-print></s:Body></s:Envelope>`,
		content,
	)

	jobData, err := escpos.ParseXML([]byte(body))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid print data"})
	}

	reply, err := s.mgr.WriteAsync(printerID, jobData)
	if err == nil {
		result := <-reply
		if !result.OK {
			err = result.Err
		}
	}
	if err != nil {
		if errors.Is(err, printer.ErrQueueFull) {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "printer queue full"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// bindJSON is a tiny helper that unmarshals the request body into v.
func bindJSON(c fiber.Ctx, v any) error {
	return json.Unmarshal(c.Body(), v)
}

func (s *Server) Stop() error {
	logger.Infof("Stopping HTTP server")
	s.running.Store(false)
	return s.app.Shutdown()
}

func (s *Server) Running() bool {
	return s.running.Load()
}
