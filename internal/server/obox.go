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
	dbUuid string
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
	prev := m.liveStatus.Load()
	changed := prev == nil || *prev != st
	m.liveStatus.Store(&st)
	if changed && m.server != nil {
		m.server.NotifyStatusChange()
	}
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
				dbUuid: odooCfg.DbUUID,
			})
			initialStatus = "connecting"
			logger.Infof("[mock] Restored Odoo credentials from storage: db=%s dbuuid=%s", odooCfg.DbURL, odooCfg.DbUUID)
		}
		m.setLiveStatus(initialStatus)
		m.registerRoutes()
		go m.deviceBrain()
	})
}

func (m *oboxModule) getStatus() OdooStatus {
	appID := m.appID()
	ipAddr := m.server.LocalAddr()

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
		}
	}
	if m.server != nil && m.server.cfg != nil && m.server.cfg.HasOdooCredentials() {
		return OdooStatus{
			AppId:           appID,
			IpAddress:       ipAddr,
			Connected:       true,
			DbURL:           m.server.cfg.GetOdooDbURL(),
			WebsocketStatus: "connecting",
		}
	}
	return OdooStatus{
		AppId:           appID,
		IpAddress:       ipAddr,
		Connected:       false,
		WebsocketStatus: "disconnected",
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

	// ── Scale endpoints (unsupported on obox-app) ──────────────────────────
	s.app.Post("/usb/v1/scale/read_scale_weight", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_scale",
			"message": "No Scale supported/needed on this host",
		})
	})

	s.app.Get("/usb/v1/scale/list", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_scale",
			"message": "No Scale supported/needed on this host",
		})
	})

	// ── Camera endpoints (unsupported on obox-app) ─────────────────────────
	s.app.Get("/usb/v1/camera/take-picture", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_camera",
			"message": "No Camera supported/needed on this host",
		})
	})

	s.app.Get("/usb/v1/camera/list", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_camera",
			"message": "No Camera supported/needed on this host",
		})
	})

	// ── Display endpoints (unsupported on obox-app) ────────────────────────
	s.app.Post("/display/v1/update-url", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_display",
			"message": "No Display supported/needed on this host",
		})
	})

	// ── WiFi endpoints (unsupported on obox-app) ───────────────────────────
	s.app.Get("/wifi/", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_wifi",
			"message": "No Wi-Fi supported/needed on this host",
		})
	})

	s.app.Get("/wifi/status", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_wifi",
			"message": "No Wi-Fi supported/needed on this host",
		})
	})

	s.app.Get("/wifi/networks", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_wifi",
			"message": "No Wi-Fi supported/needed scan on this host",
		})
	})

	s.app.Post("/wifi/connect", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_wifi",
			"message": "No Wi-Fi supported/needed on this host",
		})
	})

	// ── LED endpoints (unsupported on obox-app) ────────────────────────────
	s.app.Post("/leds/set", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_leds",
			"message": "LEDs are not supported/needed on this host",
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
					"serial": s.cfg.GetAppID(),
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
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_sos",
			"message": "Remote debug (SOS) is not supported/needed on this host",
		})
	})

	// GET /sos/v1/disable
	s.app.Get("/sos/v1/disable", func(ctx fiber.Ctx) error {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "no_sos",
			"message": "Remote debug (SOS) is not supported/needed on this host",
		})
	})

	// ── Offline connect handshake ──
	// GET /odoo/connect?db_url=...&db_uuid=...&token=...
	s.app.Get("/odoo/connect", func(ctx fiber.Ctx) error {
		dbURL := ctx.Query("db_url")
		token := ctx.Query("token")
		dbUUID := ctx.Query("db_uuid")
	
		logger.Infof("[mock] Obox offline connect received: db_url=%s, token=%s, db_uuid=%s", dbURL, token, dbUUID)
		if dbURL != "" && token != "" {
			m.device.Store(&oboxDevice{dbURL: dbURL, token: token, dbUuid: dbUUID})
			m.setLiveStatus("connecting")
			if m.server.cfg != nil {
				if err := m.server.cfg.SetOdooCredentials(dbURL, token, dbUUID); err != nil {
					logger.Warnf("[mock] Failed to save Odoo credentials to storage: %v", err)
				}
			}
			go m.callOdooOboxConnect(dbURL, token, dbUUID)
		}
		return ctx.SendStatus(fiber.StatusOK)
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
			if isDeviceNotFound(err) {
				// Server explicitly rejected this device (404 / no record).
				// Disconnect immediately and clear stored credentials.
				logger.Warnf("[mock brain] Device not found on server, disconnecting: %v", err)
				m.disconnect()
				continue
			}
			logger.Infof("[mock brain] fetchNextActions: %v", err)
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
			"serial_number": m.appID(),
			"token":         dev.token,
		},
	})

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(dev.dbURL+"/obox/get_next_actions", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d from /obox/get_next_actions", resp.StatusCode)
	}

	var rpcResp struct {
		Result []queueAction    `json:"result"`
		Error  *json.RawMessage `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, &rpcError{raw: string(*rpcResp.Error)}
	}
	return rpcResp.Result, nil
}

// rpcError carries the raw JSON-RPC error body returned by the server.
type rpcError struct{ raw string }

func (e *rpcError) Error() string {
	return "server RPC error: " + e.raw
}

// isDeviceNotFound reports whether err is an rpcError containing a 404 code,
// meaning the server has no record of this device.
func isDeviceNotFound(err error) bool {
	var rpcErr *rpcError
	if !errors.As(err, &rpcErr) {
		return false
	}
	return strings.Contains(rpcErr.raw, `"code": 404`) ||
		strings.Contains(rpcErr.raw, `"code":404`)
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
		logger.Errorf("[sos] Action remote debug enable failed: sos not supported")
		result = map[string]interface{}{
			"error":   "no_sos",
			"message": "Remote debug (SOS) is not supported/needed on this host",
		}
		m.reportActionResult(dev, action.UUID, result)
		return

	case strings.HasPrefix(actionPath, "/sos/v1/disable"):
		logger.Errorf("[sos] Action remote debug disable failed: sos not supported")
		result = map[string]interface{}{
			"error":   "no_sos",
			"message": "Remote debug (SOS) is not supported/needed on this host",
		}
		m.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/usb/v1/printer/print":
		logger.Infof("[mock brain] Action printer print: executing print simulation")
		result = map[string]string{"status": "ok"}
		m.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/usb/v1/scale/read_scale_weight":
		logger.Errorf("[scale] Action read scale weight failed: scale not supported")
		result = map[string]interface{}{
			"error":   "no_scale",
			"message": "No Scale supported/needed on this host",
		}
		m.reportActionResult(dev, action.UUID, result)
		return

	case strings.HasPrefix(actionPath, "/usb/v1/camera/take-picture"):
		logger.Errorf("[camera] Action take-picture failed: camera not supported")
		result = map[string]interface{}{
			"error":   "no_camera",
			"message": "No Camera supported/needed on this host",
		}
		m.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/display/v1/update-url":
		logger.Errorf("[display] Action display update-url failed: display not supported")
		result = map[string]interface{}{
			"error":   "no_display",
			"message": "No Display supported/needed on this host",
		}
		m.reportActionResult(dev, action.UUID, result)
		return

	case actionPath == "/leds/set":
		logger.Errorf("[leds] Action leds set failed: leds not supported")
		result = map[string]interface{}{
			"error":   "no_leds",
			"message": "LEDs are not supported/needed on this host",
		}
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
	logger.Infof("[mock brain] local action result: %v", result)
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
			"serial_number": m.appID(),
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
			"serial_number": m.appID(),
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

func (m *oboxModule) callOdooOboxConnect(dbURL, token, dbUuid string) {


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

			payload := rpcPayload{
				JSONRPC: "2.0",
				Method:  "call",
				ID:      1,
				Params: map[string]interface{}{
					"serial_number": m.appID(),
					"token":         token,
					"local_ip":      m.server.LocalAddr(),
					"services":      []string{"usb", "printer"},
				},
			}

			body, err := json.Marshal(payload)
			if err != nil {
				logger.Errorf("[mock] callOdooOboxConnect marshal error: %v", err)
				return
			}

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
				logger.Infof("[mock] Odoo /obox/connect SUCCESS on attempt %d (paired as %s)", attempt, m.appID())
				m.device.Store(&oboxDevice{
					dbURL:  dbURL,
					token:  token,
					dbUuid: dbUuid,
				})
				if m.server.cfg != nil {
					_ = m.server.cfg.SetOdooCredentials(dbURL, token, dbUuid)
				}
				m.setLiveStatus("connected")
				return
			}

			if rpcResp.Error != nil {
				logger.Warnf("[mock] /obox/connect (serial=%s) error from Odoo: %s", m.appID(), string(*rpcResp.Error))
			} else {
				logger.Warnf("[mock] /obox/connect (serial=%s) HTTP %d", m.appID(), resp.StatusCode)
			}
		
	}

	logger.Errorf("[mock] Failed to complete /obox/connect after 10 attempts")
}
