package server

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"

	"github.com/gofiber/fiber/v3"
)

func matchSerial(s1, s2 string) bool {
	return s1 != "" && s2 != "" && s1 == s2
}

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

// oboxModule encapsulates the state and behavior of the Obox mock service.
type oboxModule struct {
	server          *Server
	device          atomic.Pointer[oboxDevice]
	mockWeight      atomic.Uint64
	liveStatus      atomic.Pointer[string]
	lastContactTime atomic.Int64
}

func (m *oboxModule) setLiveStatus(st string) {
	m.liveStatus.Store(&st)
}

// ── Auto-binding registration ──────────────────────────────────────────────────

func init() {
	Register(func(s *Server) {
		m := &oboxModule{server: s}
		s.obox = m
		m.setMockWeight(1.250) // Default mock scale weight in kg
		initialStatus := "disconnected"
		if s.cfg != nil && s.cfg.HasOdooCredentials() {
			odooCfg := s.cfg.GetOdooConfig()
			m.device.Store(&oboxDevice{
				dbURL:  odooCfg.DbURL,
				token:  odooCfg.Token,
				serial: odooCfg.SerialNumber,
			})
			initialStatus = "connecting"
			logger.Infof("[mock] Restored Odoo credentials from storage: db=%s serial=%s", odooCfg.DbURL, odooCfg.SerialNumber)
		}
		m.setLiveStatus(initialStatus)
		m.registerRoutes()
		go m.deviceBrain()
	})
}

func (m *oboxModule) getStatus() OdooStatus {
	appID := m.appID()
	ipAddr := ""
	if m.server != nil {
		ipAddr = m.server.LocalAddr()
	}

	dev := m.device.Load()
	if dev != nil && dev.dbURL != "" {
		st := "connecting"
		if ptr := m.liveStatus.Load(); ptr != nil && *ptr != "" {
			st = *ptr
		}
		return OdooStatus{
			AppId:           appID,
			IpAddress:       ipAddr,
			Connected:       true,
			DbURL:           dev.dbURL,
			WebsocketStatus: st,
			Serial:          dev.serial,
		}
	}
	if m.server != nil && m.server.cfg != nil && m.server.cfg.HasOdooCredentials() {
		return OdooStatus{
			AppId:           appID,
			IpAddress:       ipAddr,
			Connected:       true,
			DbURL:           m.server.cfg.GetOdooDbURL(),
			WebsocketStatus: "connecting",
			Serial:          m.server.cfg.GetOdooSerial(),
		}
	}
	return OdooStatus{
		AppId:           appID,
		IpAddress:       ipAddr,
		Connected:       false,
		WebsocketStatus: "disconnected",
		Serial:          appID,
	}
}

func (m *oboxModule) disconnect() {
	logger.Infof("[mock] Obox disconnect triggered")
	m.device.Store(nil)
	m.setLiveStatus("disconnected")
	if m.server != nil && m.server.cfg != nil {
		if err := m.server.cfg.ClearOdooConfig(); err != nil {
			logger.Warnf("[mock] Failed to clear Odoo credentials from storage: %v", err)
		}
	}
}

func (m *oboxModule) appID() string {
	if m.server != nil {
		return m.server.AppID()
	}
	return ""
}

func (m *oboxModule) getMockWeight() float64 {
	bits := m.mockWeight.Load()
	if bits == 0 {
		return 1.250
	}
	return math.Float64frombits(bits)
}

func (m *oboxModule) setMockWeight(w float64) {
	m.mockWeight.Store(math.Float64bits(w))
}

func (m *oboxModule) registerRoutes() {
	s := m.server

	// ── Obox USB/LAN device alias ──────────────────────────────────────────
	// obox test_obox_device.js sends ePOS jobs directly to:
	//   http://{local_ip}/usb/v1/printer/{identifier}/cgi-bin/epos/service.cgi
	s.app.Post("/usb/v1/printer/:printerId/cgi-bin/epos/service.cgi", func(ctx fiber.Ctx) error {
		id := ctx.Params("printerId")
		cleanID := strings.TrimPrefix(id, "ipp_")
		cleanID = strings.TrimPrefix(cleanID, "usb_")
		logger.Infof("[mock] Obox USB ePOS print for printer: %s (clean: %s)", id, cleanID)

		if cleanID == "virtual_pos_printer" || cleanID == "mock_device" {
			logger.Infof("[mock] Virtual printer ePOS print success for: %s", id)
			return ctx.XML(EPOSResponse{Success: true, Code: "", Status: ""})
		}

		if s.mgr != nil {
			return printData(s.mgr, ctx, cleanID)
		}
		return ctx.XML(EPOSResponse{Success: true, Code: "", Status: ""})
	})

	// ── Obox USB printer generic print endpoint ────────────────────────────
	// Called by printer.obox_print() and TestOboxDevice.testPrinter()
	s.app.Post("/usb/v1/printer/print", func(ctx fiber.Ctx) error {
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

		if req.Identifier != "" && s.mgr != nil {
			cleanID := strings.TrimPrefix(req.Identifier, "usb_")
			cleanID = strings.TrimPrefix(cleanID, "ipp_")
			if req.Document != "" {
				_ = printLabel(s.mgr, ctx, cleanID)
			}
		}

		return ctx.JSON(map[string]string{"status": "ok", "success": "Sent print job"})
	})

	// ── Obox USB printer cashbox drawer ────────────────────────────────────
	s.app.Get("/usb/v1/printer/open-cashbox", func(ctx fiber.Ctx) error {
		identifier := ctx.Query("identifier")
		logger.Infof("[mock] /usb/v1/printer/open-cashbox for printer '%s'", identifier)
		return ctx.JSON(map[string]string{"success": "Opened cashbox"})
	})

	// ── Obox USB printer list ──────────────────────────────────────────────
	s.app.Get("/usb/v1/printer/list", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] /usb/v1/printer/list")
		var list [][]string
		for _, dev := range m.buildDeviceList() {
			if dev.Type == "printer" {
				list = append(list, []string{dev.Identifier, dev.Name})
			}
		}
		return ctx.JSON(list)
	})

	// ── Simulated Scale endpoints ──────────────────────────────────────────
	// Called by TestOboxDevice.testScale(), pos_iot, obox_mrp quality check
	s.app.Post("/usb/v1/scale/read_scale_weight", func(ctx fiber.Ctx) error {
		var req struct {
			Identifier string  `json:"identifier"`
			UnitPrice  float64 `json:"unit_price"`
		}
		_ = ctx.Bind().JSON(&req)
		weight := m.getMockWeight()
		logger.Infof("[mock] /usb/v1/scale/read_scale_weight for '%s' -> %.3f kg", req.Identifier, weight)
		return ctx.JSON(map[string]interface{}{
			"weight": weight,
			"unit":   "kg",
			"status": "ok",
		})
	})

	s.app.Get("/usb/v1/scale/list", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] /usb/v1/scale/list")
		return ctx.JSON([][]string{})
	})

	// ── Camera endpoints ───────────────────────────────────────────────────
	s.app.Get("/usb/v1/camera/take-picture", func(ctx fiber.Ctx) error {
		identifier := ctx.Query("identifier")
		logger.Errorf("[camera] /usb/v1/camera/take-picture failed: camera device not supported for identifier '%s'", identifier)
		return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("camera device not supported for identifier '%s'", identifier),
		})
	})

	s.app.Get("/usb/v1/camera/list", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] /usb/v1/camera/list")
		return ctx.JSON([][]string{})
	})

	// ── Simulated Display endpoints ────────────────────────────────────────
	s.app.Post("/display/v1/update-url", func(ctx fiber.Ctx) error {
		var req struct {
			URL string `json:"url"`
		}
		_ = ctx.Bind().JSON(&req)
		logger.Infof("[mock] /display/v1/update-url -> %s", req.URL)
		return ctx.JSON(map[string]string{"status": "success"})
	})

	// ── Simulated WiFi endpoints ───────────────────────────────────────────
	s.app.Get("/wifi/", func(ctx fiber.Ctx) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	s.app.Get("/wifi/status", func(ctx fiber.Ctx) error {
		return ctx.JSON(map[string]interface{}{
			"wired": map[string]string{"status": "connected", "ip": m.server.LocalAddr()},
			"wifi":  map[string]interface{}{"status": "disconnected", "ip": nil},
		})
	})

	s.app.Get("/wifi/networks", func(ctx fiber.Ctx) error {
		return ctx.JSON([]map[string]interface{}{
			{"ssid": "Mock_WiFi_Network", "signal": 95},
		})
	})

	s.app.Post("/wifi/connect", func(ctx fiber.Ctx) error {
		return ctx.JSON(map[string]string{"status": "connected"})
	})

	// ── Simulated LED endpoints ────────────────────────────────────────────
	s.app.Post("/leds/set", func(ctx fiber.Ctx) error {
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	// ── Mock IoT Proxy endpoints ───────────────────────────────────────────

	// GET /odoo-enterprise/iot/discover-boxes
	// Called by discover_obox.js → rpc(ODOO_PROXY_DISCOVER_BOXES_ENDPOINT)
	s.app.Get("/odoo-enterprise/iot/discover-boxes", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] IoT discover-boxes")
		appID := m.appID()

		reqSerial := ctx.Query("serial")
		if reqSerial == "" {
			reqSerial = ctx.Query("serial_number")
		}

		if reqSerial != "" && !matchSerial(reqSerial, appID) {
			logger.Warnf("[mock] discover-boxes: requested serial '%s' does not match app ID '%s'", reqSerial, appID)
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("serial number '%s' does not match application ID '%s'", reqSerial, appID),
			})
		}

		boxes := []map[string]string{
			{"serial_number": appID, "pairing_code": "MOCKPAIR01"},
		}
		return ctx.JSON(boxes)
	})

	// POST /odoo-enterprise/iot/connect-db
	// Called by obox_obox.py → pair_obox()
	s.app.Post("/odoo-enterprise/iot/connect-db", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] IoT connect-db")

		var body struct {
			Params struct {
				DatabaseURL  string `json:"database_url"`
				Token        string `json:"token"`
				SerialNumber string `json:"serial_number"`
				Serial       string `json:"serial"`
			} `json:"params"`
		}

		if err := json.Unmarshal(ctx.Body(), &body); err != nil ||
			body.Params.DatabaseURL == "" || body.Params.Token == "" {
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "database_url and token are required",
			})
		}

		appID := m.appID()
		reqSerial := body.Params.SerialNumber
		if reqSerial == "" {
			reqSerial = body.Params.Serial
		}

		if reqSerial != "" && !matchSerial(reqSerial, appID) {
			logger.Warnf("[mock] connect-db: requested serial '%s' does not match app ID '%s'", reqSerial, appID)
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("serial number '%s' does not match application ID '%s'", reqSerial, appID),
			})
		}

		serial := appID
		dbURL := body.Params.DatabaseURL
		token := body.Params.Token

		if m.server.cfg != nil {
			if err := m.server.cfg.SetOdooCredentials(dbURL, token, serial, ""); err != nil {
				logger.Warnf("[mock] Failed to save Odoo credentials to storage: %v", err)
			}
		}

		logger.Infof("[mock] connect-db: got credentials db=%s serial=%s — seeding brain + calling back /obox/connect", dbURL, serial)
		go m.callOdooOboxConnect(dbURL, token, serial)

		return ctx.JSON(map[string]interface{}{
			"result": []map[string]string{
				{"serial_number": serial},
			},
		})
	})

	// ── Mock Obox local device endpoints ──────────────────────────────────
	// These are hit by obox OWL widgets when connecting to the server.

	// GET /odoo/
	// OboxStatus widget LAN health check
	s.app.Get("/odoo/", func(ctx fiber.Ctx) error {
		logger.Debug("[mock] Obox LAN health check /odoo/")
		dev := m.device.Load()
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
	s.app.Get("/odoo/health", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox /odoo/health ping")
		dev := m.device.Load()
		if dev != nil {
			go m.callOdooPing(dev)
		}
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	// GET /odoo/restart
	// Ignore restart obox call, return success, do NOT attempt to restart eposproxy
	s.app.Get("/odoo/restart", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox restart — ignored, returning success")
		return ctx.JSON(map[string]string{"status": "restarted"})
	})

	// GET /odoo/disconnect
	s.app.Get("/odoo/disconnect", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox disconnect — clearing device credentials")
		m.device.Store(nil)
		m.setLiveStatus("disconnected")
		if m.server.cfg != nil {
			if err := m.server.cfg.ClearOdooConfig(); err != nil {
				logger.Warnf("[mock] Failed to clear Odoo credentials from storage: %v", err)
			}
		}
		return ctx.JSON(map[string]string{"status": "disconnected"})
	})

	// GET /odoo/discover_devices
	s.app.Get("/odoo/discover_devices", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox discover_devices")
		devices := m.buildDeviceList()
		return ctx.JSON(devices)
	})

	// GET /sos/v1/enable
	s.app.Get("/sos/v1/enable", func(ctx fiber.Ctx) error {
		token := ctx.Query("token")
		logger.Infof("[mock] Obox remote debug enable (token=%s)", token)
		return ctx.JSON(map[string]string{
			"status": "Remote Debug is enabled, Odoo support team can now access the device.",
		})
	})

	// GET /sos/v1/disable
	s.app.Get("/sos/v1/disable", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox remote debug disable")
		return ctx.JSON(map[string]string{"status": "Remote Debug is disabled."})
	})

	// ── Offline connect handshake ──────────────────────────────────────────
	// GET /odoo/connect?db_url=...&db_uuid=...&token=...&serial=...
	s.app.Get("/odoo/connect", func(ctx fiber.Ctx) error {
		dbURL := ctx.Query("db_url")
		token := ctx.Query("token")
		dbUUID := ctx.Query("db_uuid")
		reqSerial := ctx.Query("serial_number")
		if reqSerial == "" {
			reqSerial = ctx.Query("serial")
		}

		appID := m.appID()
		if reqSerial != "" && !matchSerial(reqSerial, appID) {
			logger.Warnf("[mock] /odoo/connect: requested serial '%s' does not match app ID '%s'", reqSerial, appID)
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("serial number '%s' does not match application ID '%s'", reqSerial, appID),
			})
		}

		serial := appID
		if reqSerial != "" {
			serial = reqSerial
		}

		logger.Infof("[mock] Obox offline connect received: db_url=%s, token=%s, serial=%s, db_uuid=%s", dbURL, token, serial, dbUUID)
		if dbURL != "" && token != "" {
			m.device.Store(&oboxDevice{dbURL: dbURL, token: token, serial: serial})
			m.setLiveStatus("connecting")
			if m.server.cfg != nil {
				if err := m.server.cfg.SetOdooCredentials(dbURL, token, serial, dbUUID); err != nil {
					logger.Warnf("[mock] Failed to save Odoo credentials to storage: %v", err)
				}
			}
			go m.callOdooOboxConnect(dbURL, token, serial)
		}
		return ctx.SendStatus(fiber.StatusOK)
	})

	// ── /mock/* management endpoints ─────────────────────────────────────
	s.app.Post("/mock/connect", func(ctx fiber.Ctx) error {
		var body struct {
			DbURL        string `json:"db_url"`
			Token        string `json:"token"`
			Serial       string `json:"serial"`
			SerialNumber string `json:"serial_number"`
		}
		if err := ctx.Bind().JSON(&body); err != nil || body.DbURL == "" || body.Token == "" {
			return ctx.Status(fiber.StatusBadRequest).JSON(map[string]string{"error": "db_url and token are required"})
		}

		reqSerial := body.Serial
		if reqSerial == "" {
			reqSerial = body.SerialNumber
		}

		appID := m.appID()
		if reqSerial != "" && !matchSerial(reqSerial, appID) {
			logger.Warnf("[mock] /mock/connect: requested serial '%s' does not match app ID '%s'", reqSerial, appID)
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("serial number '%s' does not match application ID '%s'", reqSerial, appID),
			})
		}

		serial := appID
		if reqSerial != "" {
			serial = reqSerial
		}

		m.device.Store(&oboxDevice{dbURL: body.DbURL, token: body.Token, serial: serial})
		m.setLiveStatus("connecting")
		if m.server.cfg != nil {
			if err := m.server.cfg.SetOdooCredentials(body.DbURL, body.Token, serial, ""); err != nil {
				logger.Warnf("[mock] Failed to save Odoo credentials to storage: %v", err)
			}
		}
		logger.Infof("[mock] Credentials registered via /mock/connect: db=%s serial=%s", body.DbURL, serial)
		go m.callOdooOboxConnect(body.DbURL, body.Token, serial)
		return ctx.JSON(map[string]string{"status": "ok", "serial": serial})
	})

	s.app.Get("/mock/status", func(ctx fiber.Ctx) error {
		dev := m.device.Load()
		if dev == nil {
			return ctx.JSON(map[string]interface{}{
				"brain":    "waiting",
				"device":   nil,
				"local_ip": m.server.LocalAddr(),
				"devices":  m.buildDeviceList(),
			})
		}
		return ctx.JSON(map[string]interface{}{
			"brain":    "polling",
			"db_url":   dev.dbURL,
			"serial":   dev.serial,
			"local_ip": m.server.LocalAddr(),
			"devices":  m.buildDeviceList(),
		})
	})

	s.app.Post("/mock/scale", func(ctx fiber.Ctx) error {
		var body struct {
			Weight float64 `json:"weight"`
		}
		if err := ctx.Bind().JSON(&body); err != nil {
			return ctx.Status(fiber.StatusBadRequest).JSON(map[string]string{"error": "invalid json"})
		}
		m.setMockWeight(body.Weight)
		logger.Infof("[mock] Mock scale weight set to %.3f kg", body.Weight)
		return ctx.JSON(map[string]interface{}{"status": "ok", "weight": body.Weight})
	})
}

// ── Mock device brain ──────────────────────────────────────────────────────────

func (m *oboxModule) deviceBrain() {
	logger.Infof("[mock brain] Background polling worker started")
	for {
		time.Sleep(2 * time.Second)

		dev := m.device.Load()
		if dev == nil {
			m.setLiveStatus("disconnected")
			continue
		}

		actions, err := m.fetchNextActions(dev)
		if err != nil {
			logger.Debugf("[mock brain] fetchNextActions: %v", err)
			last := m.lastContactTime.Load()
			if last == 0 || time.Since(time.UnixMilli(last)) > 8*time.Second {
				m.setLiveStatus("disconnected")
			} else {
				m.setLiveStatus("connecting")
			}
			continue
		}

		m.setLiveStatus("connected")
		m.lastContactTime.Store(time.Now().UnixMilli())

		for _, action := range actions {
			go m.executeAction(dev, action)
		}
	}
}

type queueAction struct {
	UUID    string                 `json:"uuid"`
	Payload map[string]interface{} `json:"payload"`
}

// fetchNextActions calls /obox/get_next_actions on Odoo and returns pending actions.
func (m *oboxModule) fetchNextActions(dev *oboxDevice) ([]queueAction, error) {
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
func (m *oboxModule) executeAction(dev *oboxDevice, action queueAction) {
	rawURL, _ := action.Payload["url"].(string)
	method, _ := action.Payload["method"].(string)
	payload := action.Payload["payload"]

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
		m.reportActionResult(dev, action.UUID, result)
		go m.callOdooPing(dev)
		return

	case actionPath == "/odoo/restart":
		// Ignore restart obox call, return success, do NOT attempt to restart eposproxy
		logger.Infof("[mock brain] Action restart: ignored, returning success")
		result = map[string]string{"status": "restarted"}
		m.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/odoo/disconnect":
		logger.Infof("[mock brain] Action disconnect: returning success")
		m.device.Store(nil)
		result = map[string]string{"status": "disconnected"}
		m.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/odoo/discover_devices":
		logger.Infof("[mock brain] Action discover_devices: fetching device list")
		devices := m.buildDeviceList()
		devicesJSON, err := json.Marshal(devices)
		if err == nil {
			// Send as string so Odoo's json.loads(self.result) works reliably
			result = string(devicesJSON)
		} else {
			result = "[]"
		}
		m.reportActionResult(dev, action.UUID, result)
		return

	case strings.HasPrefix(actionPath, "/sos/v1/enable"):
		result = "enabled"
		m.reportActionResult(dev, action.UUID, result)
		return

	case strings.HasPrefix(actionPath, "/sos/v1/disable"):
		result = "disabled"
		m.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/usb/v1/printer/print":
		logger.Infof("[mock brain] Action printer print: executing print simulation")
		result = map[string]string{"status": "ok"}
		m.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/usb/v1/scale/read_scale_weight":
		logger.Infof("[mock brain] Action read scale weight")
		result = map[string]interface{}{
			"weight": m.getMockWeight(),
			"unit":   "kg",
			"status": "ok",
		}
		m.reportActionResult(dev, action.UUID, result)
		return

	case strings.HasPrefix(actionPath, "/usb/v1/camera/take-picture"):
		logger.Errorf("[camera] Action take-picture failed: camera not supported")
		result = map[string]interface{}{
			"error": "camera device not supported",
		}
		m.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/display/v1/update-url":
		logger.Infof("[mock brain] Action display update-url")
		result = map[string]string{"status": "success"}
		m.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/leds/set":
		logger.Infof("[mock brain] Action leds set")
		result = map[string]string{"status": "ok"}
		m.reportActionResult(dev, action.UUID, result)
		return

	default:
		result = m.dispatchLocalAction(rawURL, method, payload)
		m.reportActionResult(dev, action.UUID, result)
	}
}

// dispatchLocalAction calls the mock device endpoint and returns the result.
func (m *oboxModule) dispatchLocalAction(path, method string, payload interface{}) interface{} {
	base := fmt.Sprintf("http://127.0.0.1:%d", m.server.Port)
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
func (m *oboxModule) reportActionResult(dev *oboxDevice, uuid string, result interface{}) {
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
func (m *oboxModule) callOdooPing(dev *oboxDevice) {
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

func (m *oboxModule) buildDeviceList() []oboxDeviceEntry {
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
	if m.server.cfg != nil {
		for _, lan := range printer.ListLANPrinters(m.server.cfg) {
			devices = append(devices, oboxDeviceEntry{
				Name:       fmt.Sprintf("Network Printer %s", lan.IP),
				Identifier: "ipp_" + lan.Id,
				Type:       "printer",
			})
		}
	}

	logger.Infof("[mock] buildDeviceList: %d devices found", len(devices))
	return devices
}

// ── Odoo handshake callback ────────────────────────────────────────────────────

func (m *oboxModule) callOdooOboxConnect(dbURL, token, serial string) {
	if serial == "" {
		serial = m.appID()
	}

	cleanSerial := strings.TrimPrefix(strings.TrimPrefix(serial, "ODO-"), "ODO")

	// Generate all candidate serial variations for Odoo pairing
	candidateMap := make(map[string]bool)
	var candidateSerials []string

	addCandidate := func(s string) {
		if s != "" && !candidateMap[s] {
			candidateMap[s] = true
			candidateSerials = append(candidateSerials, s)
		}
	}

	addCandidate(serial)
	addCandidate(cleanSerial)
	addCandidate("ODO-" + cleanSerial)
	addCandidate("ODO" + cleanSerial)

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
					"local_ip":      m.server.LocalAddr(),
					"services":      []string{"usb", "printer", "wifi"},
				},
			}

			body, err := json.Marshal(payload)
			if err != nil {
				logger.Errorf("[mock] callOdooOboxConnect marshal error: %v", err)
				return
			}

			logger.Infof("[mock] Calling back Odoo at %s (serial=%s, attempt %d/10)", endpoint, candSerial, attempt)
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
				m.device.Store(&oboxDevice{
					dbURL:  dbURL,
					token:  token,
					serial: candSerial,
				})
				if m.server.cfg != nil {
					_ = m.server.cfg.SetOdooCredentials(dbURL, token, candSerial, "")
				}
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
