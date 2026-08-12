package server

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"epos-proxy/config"
	"epos-proxy/escpos"
	"epos-proxy/logger"
	"epos-proxy/printer"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

// ── Mock constants ─────────────────────────────────────────────────────────────

const (
	mockSerialNumber   = "12345"
	virtualPrinterID   = "usb_virtual_pos_printer"
	virtualPrinterName = "Virtual POS Receipt Printer"

	// Minimal valid base64 1x1 JPEG image for mock camera previews
	sampleJPEGImageBase64 = "/9j/4AAQSkZJRgABAQEASABIAAD/2wBDAP//////////////////////////////////////////////////////////////////////////////////////wgALCAABAAEBAREA/8QAFBABAAAAAAAAAAAAAAAAAAAAAP/aAAgBAQABPxA="
)

// ── Types ──────────────────────────────────────────────────────────────────────

type EPOSResponse struct {
	XMLName xml.Name `xml:"response"`
	Success bool     `xml:"success,attr"`
	Code    string   `xml:"code,attr"`
	Status  string   `xml:"status,attr"`
}

// oboxDevice holds the credentials needed to poll Odoo on behalf of the
// mock obox device. Fields are set once Odoo pairing is initiated.
type oboxDevice struct {
	dbURL  string
	token  string
	serial string
}

type Server struct {
	app     *fiber.App
	Port    int
	running atomic.Bool

	// mgr is kept so the device brain can list real printers
	mgr *printer.Manager
	// cfg is kept for LAN printer listing
	cfg *config.Manager

	// device credentials are written once pairing occurs and read
	// by the polling goroutine – atomic pointer keeps this race-free.
	device atomic.Pointer[oboxDevice]

	// mockWeight stores float64 bits for simulated scale reading
	mockWeight atomic.Uint64
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

	s := &Server{app: app, Port: port, mgr: mgr, cfg: conf}
	s.setMockWeight(1.250) // Default mock scale weight in kg

	// ── ePOS print routes ────────────────────────────────────────────────
	app.Post("/p/:printerId/cgi-bin/epos/service.cgi", func(ctx fiber.Ctx) error {
		id := ctx.Params("printerId")
		logger.Debugf("Print request received for printer: %s", id)
		return s.handlePrintData(ctx, id)
	})

	app.Post("/cgi-bin/epos/service.cgi", func(ctx fiber.Ctx) error {
		logger.Debugf("Print request received (auto printer selection)")
		return s.handlePrintData(ctx, "")
	})

	app.Post("/p/:printerId/pstprnt", func(ctx fiber.Ctx) error {
		id := ctx.Params("printerId")
		logger.Debugf("Label print request received for printer: %s", id)
		return printLabel(mgr, ctx, id)
	})

	// ── Obox USB/LAN device alias ──────────────────────────────────────────
	// obox test_obox_device.js sends ePOS jobs directly to:
	//   http://{local_ip}/usb/v1/printer/{identifier}/cgi-bin/epos/service.cgi
	app.Post("/usb/v1/printer/:printerId/cgi-bin/epos/service.cgi", func(ctx fiber.Ctx) error {
		id := ctx.Params("printerId")
		rawID := id
		id = strings.TrimPrefix(id, "ipp_")
		id = strings.TrimPrefix(id, "usb_")
		logger.Infof("[mock] Obox USB ePOS print for printer: %s (raw: %s)", id, rawID)
		return s.handlePrintData(ctx, rawID)
	})

	// ── Obox USB printer generic print endpoint ────────────────────────────
	// Called by printer.obox_print() and TestOboxDevice.testPrinter()
	app.Post("/usb/v1/printer/print", func(ctx fiber.Ctx) error {
		var req struct {
			Identifier string `json:"identifier"`
			Document   string `json:"document"`
			Receipt    string `json:"receipt"`
			Duplex     bool   `json:"duplex"`
		}
		if err := ctx.Bind().JSON(&req); err != nil {
			logger.Warnf("[mock] /usb/v1/printer/print JSON parse error: %v", err)
		}
		logger.Infof("[mock] /usb/v1/printer/print job for printer '%s' (docLen=%d, receiptLen=%d)",
			req.Identifier, len(req.Document), len(req.Receipt))

		// If real printer exists, attempt print via manager
		if req.Identifier != "" && req.Identifier != virtualPrinterID && s.mgr != nil {
			cleanID := strings.TrimPrefix(req.Identifier, "usb_")
			cleanID = strings.TrimPrefix(cleanID, "ipp_")
			if req.Document != "" {
				_ = printLabel(s.mgr, ctx, cleanID)
			}
		}

		return ctx.JSON(map[string]string{"status": "ok", "success": "Sent print job"})
	})

	// ── Obox USB printer cashbox drawer ────────────────────────────────────
	app.Get("/usb/v1/printer/open-cashbox", func(ctx fiber.Ctx) error {
		identifier := ctx.Query("identifier")
		logger.Infof("[mock] /usb/v1/printer/open-cashbox for printer '%s'", identifier)
		return ctx.JSON(map[string]string{"success": "Opened cashbox"})
	})

	// ── Obox USB printer list ──────────────────────────────────────────────
	app.Get("/usb/v1/printer/list", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] /usb/v1/printer/list")
		var list [][]string
		for _, dev := range s.buildDeviceList() {
			if dev.Type == "printer" {
				list = append(list, []string{dev.Identifier, dev.Name})
			}
		}
		return ctx.JSON(list)
	})

	// ── Simulated Scale endpoints ──────────────────────────────────────────
	// Called by TestOboxDevice.testScale(), pos_iot, obox_mrp quality check
	app.Post("/usb/v1/scale/read_scale_weight", func(ctx fiber.Ctx) error {
		var req struct {
			Identifier string  `json:"identifier"`
			UnitPrice  float64 `json:"unit_price"`
		}
		_ = ctx.Bind().JSON(&req)
		weight := s.getMockWeight()
		logger.Infof("[mock] /usb/v1/scale/read_scale_weight for '%s' -> %.3f kg", req.Identifier, weight)
		return ctx.JSON(map[string]interface{}{
			"weight": weight,
			"unit":   "kg",
			"status": "ok",
		})
	})

	app.Get("/usb/v1/scale/list", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] /usb/v1/scale/list")
		return ctx.JSON([][]string{})
	})

	// ── Simulated Camera endpoints ─────────────────────────────────────────
	// Called by TestOboxDevice.testCamera(), obox_mrp picture preview
	app.Get("/usb/v1/camera/take-picture", func(ctx fiber.Ctx) error {
		identifier := ctx.Query("identifier")
		logger.Infof("[mock] /usb/v1/camera/take-picture for '%s'", identifier)
		return ctx.JSON(map[string]string{
			"image": sampleJPEGImageBase64,
		})
	})

	app.Get("/usb/v1/camera/list", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] /usb/v1/camera/list")
		return ctx.JSON([][]string{})
	})

	// ── Simulated Display endpoints ────────────────────────────────────────
	// Called by customer display / kiosk
	app.Post("/display/v1/update-url", func(ctx fiber.Ctx) error {
		var req struct {
			URL string `json:"url"`
		}
		_ = ctx.Bind().JSON(&req)
		logger.Infof("[mock] /display/v1/update-url -> %s", req.URL)
		return ctx.JSON(map[string]string{"status": "success"})
	})

	// ── Simulated WiFi endpoints ───────────────────────────────────────────
	app.Get("/wifi/", func(ctx fiber.Ctx) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	app.Get("/wifi/status", func(ctx fiber.Ctx) error {
		return ctx.JSON(map[string]interface{}{
			"wired": map[string]string{"status": "connected", "ip": s.getLocalIP()},
			"wifi":  map[string]interface{}{"status": "disconnected", "ip": nil},
		})
	})

	app.Get("/wifi/networks", func(ctx fiber.Ctx) error {
		return ctx.JSON([]map[string]interface{}{
			{"ssid": "Mock_WiFi_Network", "signal": 95},
		})
	})

	app.Post("/wifi/connect", func(ctx fiber.Ctx) error {
		return ctx.JSON(map[string]string{"status": "connected"})
	})

	// ── Simulated LED endpoints ────────────────────────────────────────────
	app.Post("/leds/set", func(ctx fiber.Ctx) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	// ── Mock IoT Proxy endpoints ───────────────────────────────────────────

	// GET /odoo-enterprise/iot/discover-boxes
	// Called by discover_obox.js → rpc(ODOO_PROXY_DISCOVER_BOXES_ENDPOINT)
	app.Get("/odoo-enterprise/iot/discover-boxes", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] IoT discover-boxes")
		boxes := []map[string]string{
			{"serial_number": "ODO-" + mockSerialNumber, "pairing_code": "MOCKPAIR01"},
			{"serial_number": mockSerialNumber, "pairing_code": "MOCKPAIR01"},
		}
		return ctx.JSON(boxes)
	})

	// POST /odoo-enterprise/iot/connect-db
	// Called by obox_obox.py → pair_obox()
	app.Post("/odoo-enterprise/iot/connect-db", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] IoT connect-db")

		var body struct {
			Params struct {
				DatabaseURL string `json:"database_url"`
				Token       string `json:"token"`
			} `json:"params"`
		}
		if err := json.Unmarshal(ctx.Body(), &body); err == nil &&
			body.Params.DatabaseURL != "" && body.Params.Token != "" {
			dbURL := body.Params.DatabaseURL
			token := body.Params.Token
			logger.Infof("[mock] connect-db: got credentials db=%s — seeding brain + calling back /obox/connect", dbURL)
			go s.callOdooOboxConnect(dbURL, token, mockSerialNumber)
		} else {
			logger.Warn("[mock] connect-db: could not parse credentials from request body")
		}

		return ctx.JSON(map[string]interface{}{
			"result": []map[string]string{
				{"serial_number": mockSerialNumber},
			},
		})
	})

	// ── Mock Obox local device endpoints ──────────────────────────────────
	// These are hit by obox OWL widgets when local_ip == s.getLocalIP().

	// GET /odoo/
	// OboxStatus widget LAN health check
	app.Get("/odoo/", func(ctx fiber.Ctx) error {
		logger.Debug("[mock] Obox LAN health check /odoo/")
		dev := s.device.Load()
		if dev != nil {
			return ctx.JSON(map[string]interface{}{
				"status": "configured",
				"data": map[string]string{
					"serial": dev.serial,
					"db_url": dev.dbURL,
				},
			})
		}
		return ctx.JSON(map[string]interface{}{
			"status": "configured",
			"data":   nil,
		})
	})

	// GET /odoo/health (ping)
	app.Get("/odoo/health", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox /odoo/health ping")
		dev := s.device.Load()
		if dev != nil {
			go s.callOdooPing(dev)
		}
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	// GET /odoo/restart
	// Ignore restart obox call, return success, do NOT attempt to restart eposproxy
	app.Get("/odoo/restart", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox restart — ignored, returning success")
		return ctx.JSON(map[string]string{"status": "restarted"})
	})

	// GET /odoo/disconnect
	app.Get("/odoo/disconnect", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox disconnect — clearing device credentials")
		s.device.Store(nil)
		return ctx.JSON(map[string]string{"status": "disconnected"})
	})

	// GET /odoo/discover_devices
	app.Get("/odoo/discover_devices", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox discover_devices")
		devices := s.buildDeviceList()
		return ctx.JSON(devices)
	})

	// GET /sos/v1/enable
	app.Get("/sos/v1/enable", func(ctx fiber.Ctx) error {
		token := ctx.Query("token")
		logger.Infof("[mock] Obox remote debug enable (token=%s)", token)
		return ctx.JSON(map[string]string{
			"status": "Remote Debug is enabled, Odoo support team can now access the device.",
		})
	})

	// GET /sos/v1/disable
	app.Get("/sos/v1/disable", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox remote debug disable")
		return ctx.JSON(map[string]string{"status": "Remote Debug is disabled."})
	})

	// ── Offline connect handshake ──────────────────────────────────────────
	// GET /odoo/connect?db_url=...&db_uuid=...&token=...
	app.Get("/odoo/connect", func(ctx fiber.Ctx) error {
		dbURL := ctx.Query("db_url")
		token := ctx.Query("token")
		logger.Infof("[mock] Obox offline connect received: db_url=%s, token=%s", dbURL, token)
		if dbURL != "" && token != "" {
			go s.callOdooOboxConnect(dbURL, token, mockSerialNumber)
		}
		return ctx.SendStatus(fiber.StatusOK)
	})

	// ── /mock/* management endpoints ─────────────────────────────────────
	app.Post("/mock/connect", func(ctx fiber.Ctx) error {
		var body struct {
			DbURL  string `json:"db_url"`
			Token  string `json:"token"`
			Serial string `json:"serial"`
		}
		if err := ctx.Bind().JSON(&body); err != nil || body.DbURL == "" || body.Token == "" {
			return ctx.Status(fiber.StatusBadRequest).JSON(map[string]string{"error": "db_url and token are required"})
		}
		serial := body.Serial
		if serial == "" {
			serial = mockSerialNumber
		}
		s.device.Store(&oboxDevice{dbURL: body.DbURL, token: body.Token, serial: serial})
		logger.Infof("[mock] Credentials registered via /mock/connect: db=%s serial=%s", body.DbURL, serial)
		go s.callOdooOboxConnect(body.DbURL, body.Token, serial)
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	app.Get("/mock/status", func(ctx fiber.Ctx) error {
		dev := s.device.Load()
		if dev == nil {
			return ctx.JSON(map[string]interface{}{
				"brain":    "waiting",
				"device":   nil,
				"local_ip": s.getLocalIP(),
				"devices":  s.buildDeviceList(),
			})
		}
		return ctx.JSON(map[string]interface{}{
			"brain":    "polling",
			"db_url":   dev.dbURL,
			"serial":   dev.serial,
			"local_ip": s.getLocalIP(),
			"devices":  s.buildDeviceList(),
		})
	})

	app.Post("/mock/scale", func(ctx fiber.Ctx) error {
		var body struct {
			Weight float64 `json:"weight"`
		}
		if err := ctx.Bind().JSON(&body); err != nil {
			return ctx.Status(fiber.StatusBadRequest).JSON(map[string]string{"error": "invalid json"})
		}
		s.setMockWeight(body.Weight)
		logger.Infof("[mock] Mock scale weight set to %.3f kg", body.Weight)
		return ctx.JSON(map[string]interface{}{"status": "ok", "weight": body.Weight})
	})

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

	// Start mock device brain that polls Odoo for queue actions
	go s.deviceBrain()

	return s
}

// ── Dynamic Local IP ──────────────────────────────────────────────────────────

func (s *Server) getLocalIP() string {
	return fmt.Sprintf("127.0.0.1:%d", s.Port)
}

func (s *Server) getMockWeight() float64 {
	bits := s.mockWeight.Load()
	if bits == 0 {
		return 1.250
	}
	return math.Float64frombits(bits)
}

func (s *Server) setMockWeight(w float64) {
	s.mockWeight.Store(math.Float64bits(w))
}

// ── Mock device brain ──────────────────────────────────────────────────────────

func (s *Server) deviceBrain() {
	logger.Infof("[mock brain] Background polling worker started")
	for {
		time.Sleep(2 * time.Second)

		dev := s.device.Load()
		if dev == nil {
			continue
		}

		actions, err := s.fetchNextActions(dev)
		if err != nil {
			logger.Debugf("[mock brain] fetchNextActions: %v", err)
			continue
		}

		for _, action := range actions {
			go s.executeAction(dev, action)
		}
	}
}

type queueAction struct {
	UUID    string                 `json:"uuid"`
	Payload map[string]interface{} `json:"payload"`
}

// fetchNextActions calls /obox/get_next_actions on Odoo and returns pending actions.
func (s *Server) fetchNextActions(dev *oboxDevice) ([]queueAction, error) {
	type rpcPayload struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		ID      int         `json:"id"`
		Params  interface{} `json:"params"`
	}
	body, _ := json.Marshal(rpcPayload{
		JSONRPC: "2.0",
		Method:  "call",
		ID:      1,
		Params: map[string]string{
			"serial_number": dev.serial,
			"token":         dev.token,
		},
	})

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(dev.dbURL+"/obox/get_next_actions", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rpcResp struct {
		Result []queueAction `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	return rpcResp.Result, nil
}

// executeAction processes one queue action and reports the result back to Odoo.
func (s *Server) executeAction(dev *oboxDevice, action queueAction) {
	rawURL, _ := action.Payload["url"].(string)
	method, _ := action.Payload["method"].(string)
	payload := action.Payload["payload"]

	// Parse path without query parameters for clean matching
	actionPath := rawURL
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Path != "" {
		actionPath = parsed.Path
	}

	logger.Infof("[mock brain] Executing queue action uuid=%s path=%s method=%s", action.UUID, actionPath, method)

	var result interface{}

	switch {
	case actionPath == "/odoo/health":
		logger.Infof("[mock brain] Action health ping: returning success")
		result = map[string]string{"status": "ok"}
		s.reportActionResult(dev, action.UUID, result)
		go s.callOdooPing(dev)
		return

	case actionPath == "/odoo/restart":
		// Ignore restart obox call, return success, do NOT attempt to restart eposproxy
		logger.Infof("[mock brain] Action restart: ignored, returning success")
		result = map[string]string{"status": "restarted"}
		s.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/odoo/disconnect":
		logger.Infof("[mock brain] Action disconnect: returning success")
		s.device.Store(nil)
		result = map[string]string{"status": "disconnected"}
		s.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/odoo/discover_devices":
		logger.Infof("[mock brain] Action discover_devices: fetching device list")
		devices := s.buildDeviceList()
		devicesJSON, err := json.Marshal(devices)
		if err == nil {
			// Send as string so Odoo's json.loads(self.result) works reliably
			result = string(devicesJSON)
		} else {
			result = "[]"
		}
		s.reportActionResult(dev, action.UUID, result)
		return

	case strings.HasPrefix(actionPath, "/sos/v1/enable"):
		result = "enabled"
		s.reportActionResult(dev, action.UUID, result)
		return

	case strings.HasPrefix(actionPath, "/sos/v1/disable"):
		result = "disabled"
		s.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/usb/v1/printer/print":
		logger.Infof("[mock brain] Action printer print: executing print simulation")
		result = map[string]string{"status": "ok"}
		s.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/usb/v1/scale/read_scale_weight":
		logger.Infof("[mock brain] Action read scale weight")
		result = map[string]interface{}{
			"weight": s.getMockWeight(),
			"unit":   "kg",
			"status": "ok",
		}
		s.reportActionResult(dev, action.UUID, result)
		return

	case strings.HasPrefix(actionPath, "/usb/v1/camera/take-picture"):
		logger.Infof("[mock brain] Action take picture")
		result = map[string]interface{}{
			"image": sampleJPEGImageBase64,
		}
		s.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/display/v1/update-url":
		logger.Infof("[mock brain] Action display update-url")
		result = map[string]string{"status": "success"}
		s.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/leds/set":
		logger.Infof("[mock brain] Action leds set")
		result = map[string]string{"status": "ok"}
		s.reportActionResult(dev, action.UUID, result)
		return

	default:
		result = s.dispatchLocalAction(rawURL, method, payload)
		s.reportActionResult(dev, action.UUID, result)
	}
}

// dispatchLocalAction calls the mock device endpoint and returns the result.
func (s *Server) dispatchLocalAction(path, method string, payload interface{}) interface{} {
	base := fmt.Sprintf("http://%s", s.getLocalIP())
	fullURL := base + path

	var req *http.Request
	var err error

	switch method {
	case "POST":
		var bodyBytes []byte
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			logger.Errorf("[mock brain] marshal payload error: %v", err)
			return map[string]string{"status": "ok"}
		}
		req, err = http.NewRequest("POST", fullURL, bytes.NewReader(bodyBytes))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	default: // GET
		req, err = http.NewRequest("GET", fullURL, nil)
	}

	if err != nil {
		logger.Errorf("[mock brain] build request error: %v", err)
		return map[string]string{"status": "ok"}
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("[mock brain] local action HTTP error url=%s: %v", fullURL, err)
		return map[string]string{"status": "ok"}
	}
	defer resp.Body.Close()

	var result interface{}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil || result == nil {
		return map[string]string{"status": "ok"}
	}
	return result
}

// reportActionResult calls /obox/action_result on Odoo with the action uuid and result.
func (s *Server) reportActionResult(dev *oboxDevice, uuid string, result interface{}) {
	type rpcPayload struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		ID      int         `json:"id"`
		Params  interface{} `json:"params"`
	}
	body, _ := json.Marshal(rpcPayload{
		JSONRPC: "2.0",
		Method:  "call",
		ID:      1,
		Params: map[string]interface{}{
			"serial_number": dev.serial,
			"token":         dev.token,
			"action_uuid":   uuid,
			"result":        result,
		},
	})

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(dev.dbURL+"/obox/action_result", "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Errorf("[mock brain] action_result error for uuid %s: %v", uuid, err)
		return
	}
	defer resp.Body.Close()
	logger.Infof("[mock brain] action_result response status: %d for uuid %s", resp.StatusCode, uuid)
}

// callOdooPing calls /obox/ping on Odoo after processing a test action.
func (s *Server) callOdooPing(dev *oboxDevice) {
	type rpcPayload struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		ID      int         `json:"id"`
		Params  interface{} `json:"params"`
	}

	body, _ := json.Marshal(rpcPayload{
		JSONRPC: "2.0",
		Method:  "call",
		ID:      1,
		Params: map[string]string{
			"serial_number": dev.serial,
			"token":         dev.token,
		},
	})

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(dev.dbURL+"/obox/ping", "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Errorf("[mock brain] /obox/ping error: %v", err)
		return
	}
	defer resp.Body.Close()
	logger.Infof("[mock brain] /obox/ping response status: %d", resp.StatusCode)
}

// ── Device list helpers ────────────────────────────────────────────────────────

type oboxDeviceEntry struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
	Type       string `json:"type"`
}

func (s *Server) buildDeviceList() []oboxDeviceEntry {
	var devices []oboxDeviceEntry

	// USB printers
	infos, err := printer.ListUSBPrinters()
	if err == nil {
		for _, info := range infos.Available {
			devices = append(devices, oboxDeviceEntry{
				Name:       info.Name,
				Identifier: "usb_" + info.Id,
				Type:       "printer",
			})
		}
	} else {
		logger.Warnf("[mock] ListUSBPrinters error: %v", err)
	}

	// LAN/Network printers
	if s.cfg != nil {
		for _, lan := range printer.ListLANPrinters(s.cfg) {
			devices = append(devices, oboxDeviceEntry{
				Name:       fmt.Sprintf("Network Printer %s", lan.IP),
				Identifier: "ipp_" + lan.Id,
				Type:       "printer",
			})
		}
	}

	// Virtual POS Receipt Printer only (as requested, no other device added)
	devices = append(devices, oboxDeviceEntry{
		Name:       virtualPrinterName,
		Identifier: virtualPrinterID,
		Type:       "printer",
	})

	logger.Infof("[mock] buildDeviceList: %d devices found", len(devices))
	return devices
}

// ── Odoo handshake callback ────────────────────────────────────────────────────

func (s *Server) callOdooOboxConnect(dbURL, token, serial string) {
	if serial == "" {
		serial = mockSerialNumber
	}

	// Candidates to try in case Odoo has "12345" or "ODO-12345"
	candidateSerials := []string{serial}
	if serial == mockSerialNumber {
		candidateSerials = append(candidateSerials, "ODO-"+mockSerialNumber, "ODO"+mockSerialNumber)
	} else if len(serial) > 4 && serial[:4] == "ODO-" {
		candidateSerials = append(candidateSerials, serial[4:])
	}

	endpoint := dbURL + "/obox/connect"
	client := &http.Client{Timeout: 5 * time.Second}

	type rpcPayload struct {
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
		ID      int         `json:"id"`
		Params  interface{} `json:"params"`
	}

	for attempt := 1; attempt <= 10; attempt++ {
		time.Sleep(time.Duration(attempt*300) * time.Millisecond)

		for _, candSerial := range candidateSerials {
			payload := rpcPayload{
				JSONRPC: "2.0",
				Method:  "call",
				ID:      1,
				Params: map[string]interface{}{
					"serial_number": candSerial,
					"token":         token,
					"local_ip":      s.getLocalIP(),
					"services":      []string{"odoo", "usb", "display", "wifi", "leds", "sos"},
				},
			}

			body, err := json.Marshal(payload)
			if err != nil {
				logger.Errorf("[mock] callOdooOboxConnect marshal error: %v", err)
				return
			}

			logger.Infof("[mock] Calling back Odoo at %s (serial=%s, local_ip=%s, attempt %d/10)",
				endpoint, candSerial, s.getLocalIP(), attempt)
			resp, err := client.Post(endpoint, "application/json", bytes.NewReader(body))
			if err != nil {
				logger.Warnf("[mock] /obox/connect attempt %d connection error: %v", attempt, err)
				continue
			}

			var rpcResp struct {
				Result *json.RawMessage `json:"result"`
				Error  *json.RawMessage `json:"error"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&rpcResp)
			resp.Body.Close()

			if resp.StatusCode == 200 && rpcResp.Error == nil {
				logger.Infof("[mock] Odoo /obox/connect SUCCESS on attempt %d (paired as %s)", attempt, candSerial)
				s.device.Store(&oboxDevice{
					dbURL:  dbURL,
					token:  token,
					serial: candSerial,
				})
				return
			}

			if rpcResp.Error != nil {
				logger.Warnf("[mock] /obox/connect (serial=%s) error from Odoo: %s", candSerial, string(*rpcResp.Error))
			} else {
				logger.Warnf("[mock] /obox/connect (serial=%s) HTTP %d", candSerial, resp.StatusCode)
			}
		}
	}

	logger.Errorf("[mock] Failed to complete /obox/connect after 10 attempts")
}

// ── Print helpers ──────────────────────────────────────────────────────────────

func (s *Server) handlePrintData(ctx fiber.Ctx, printerID string) error {
	// If virtual/mock printer, simulate success directly
	cleanID := strings.TrimPrefix(printerID, "usb_")
	cleanID = strings.TrimPrefix(cleanID, "ipp_")

	if printerID == virtualPrinterID || cleanID == "virtual_pos_printer" || cleanID == "mock_device" {
		logger.Infof("[mock] Virtual printer ePOS print success for: %s", printerID)
		return ctx.XML(EPOSResponse{Success: true, Code: "", Status: ""})
	}

	// Try real printer write via manager
	if s.mgr != nil {
		return printData(s.mgr, ctx, cleanID)
	}

	// Fallback to success if no printer manager
	return ctx.XML(EPOSResponse{Success: true, Code: "", Status: ""})
}

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

// ── Server lifecycle ───────────────────────────────────────────────────────────

func (s *Server) Stop() error {
	logger.Infof("Stopping HTTP server")
	return s.app.Shutdown()
}

func (s *Server) Running() bool {
	return s.running.Load()
}
