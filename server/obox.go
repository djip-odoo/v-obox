package server

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"epos-proxy/logger"
	"epos-proxy/printer"

	"github.com/gofiber/fiber/v3"
)

// ── Mock constants ─────────────────────────────────────────────────────────────

const mockSerialNumber = "12345"
const mockLocalIP = "127.0.0.1:4545"

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
	server *Server
	device atomic.Pointer[oboxDevice]
}

// ── Auto-binding registration ──────────────────────────────────────────────────

func init() {
	Register(func(s *Server) {
		m := &oboxModule{server: s}
		m.registerRoutes()
		go m.deviceBrain()
	})
}

func (m *oboxModule) registerRoutes() {
	s := m.server

	// ── Obox USB/LAN device alias ──────────────────────────────────────────
	// obox test_obox_device.js sends ePOS jobs directly to:
	//   http://{local_ip}/usb/v1/printer/{identifier}/cgi-bin/epos/service.cgi
	s.app.Post("/usb/v1/printer/:printerId/cgi-bin/epos/service.cgi", func(ctx fiber.Ctx) error {
		id := ctx.Params("printerId")
		id = strings.TrimPrefix(id, "ipp_")
		id = strings.TrimPrefix(id, "usb_")
		logger.Infof("[mock] Obox USB ePOS print for printer: %s", id)
		return printData(s.mgr, ctx, id)
	})

	// ── Mock IoT Proxy endpoints ───────────────────────────────────────────

	// GET /odoo-enterprise/iot/discover-boxes
	// Called by discover_obox.js → rpc(ODOO_PROXY_DISCOVER_BOXES_ENDPOINT)
	s.app.Get("/odoo-enterprise/iot/discover-boxes", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] IoT discover-boxes")
		boxes := []map[string]string{
			{"serial_number": "ODO-" + mockSerialNumber, "pairing_code": "MOCKPAIR01"},
			{"serial_number": mockSerialNumber, "pairing_code": "MOCKPAIR01"},
		}
		return ctx.JSON(boxes)
	})

	// POST /odoo-enterprise/iot/connect-db
	// Called by obox_obox.py → pair_obox()
	s.app.Post("/odoo-enterprise/iot/connect-db", func(ctx fiber.Ctx) error {
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
			go m.callOdooOboxConnect(dbURL, token, mockSerialNumber)
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
	// These are hit by obox OWL widgets when local_ip == "127.0.0.1:4545".

	// GET /odoo/
	// OboxStatus widget LAN health check
	s.app.Get("/odoo/", func(ctx fiber.Ctx) error {
		logger.Debug("[mock] Obox LAN health check /odoo/")
		return ctx.SendStatus(fiber.StatusOK)
	})

	// GET /odoo/health (ping)
	s.app.Get("/odoo/health", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox /odoo/health ping")
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
		logger.Infof("[mock] Obox disconnect — returning success")
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
		logger.Infof("[mock] Obox remote debug enable")
		return ctx.JSON(map[string]string{"status": "enabled"})
	})

	// GET /sos/v1/disable
	s.app.Get("/sos/v1/disable", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox remote debug disable")
		return ctx.JSON(map[string]string{"status": "disabled"})
	})

	// ── Offline connect handshake ──────────────────────────────────────────
	// GET /odoo/connect?db_url=...&db_uuid=...&token=...
	s.app.Get("/odoo/connect", func(ctx fiber.Ctx) error {
		dbURL := ctx.Query("db_url")
		token := ctx.Query("token")
		logger.Infof("[mock] Obox offline connect received: db_url=%s, token=%s", dbURL, token)
		if dbURL != "" && token != "" {
			go m.callOdooOboxConnect(dbURL, token, mockSerialNumber)
		}
		return ctx.SendStatus(fiber.StatusOK)
	})

	// ── /mock/* management endpoints ─────────────────────────────────────
	s.app.Post("/mock/connect", func(ctx fiber.Ctx) error {
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
		m.device.Store(&oboxDevice{dbURL: body.DbURL, token: body.Token, serial: serial})
		logger.Infof("[mock] Credentials registered via /mock/connect: db=%s serial=%s", body.DbURL, serial)
		go m.callOdooOboxConnect(body.DbURL, body.Token, serial)
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	s.app.Get("/mock/status", func(ctx fiber.Ctx) error {
		dev := m.device.Load()
		if dev == nil {
			return ctx.JSON(map[string]interface{}{"brain": "waiting", "device": nil})
		}
		return ctx.JSON(map[string]interface{}{
			"brain":  "polling",
			"db_url": dev.dbURL,
			"serial": dev.serial,
		})
	})
}

// ── Mock device brain ──────────────────────────────────────────────────────────

func (m *oboxModule) deviceBrain() {
	logger.Infof("[mock brain] Background polling worker started")
	for {
		time.Sleep(2 * time.Second)

		dev := m.device.Load()
		if dev == nil {
			continue
		}

		actions, err := m.fetchNextActions(dev)
		if err != nil {
			logger.Debugf("[mock brain] fetchNextActions: %v", err)
			continue
		}

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
	url, _ := action.Payload["url"].(string)
	method, _ := action.Payload["method"].(string)
	payload := action.Payload["payload"]

	logger.Infof("[mock brain] Executing queue action uuid=%s url=%s method=%s", action.UUID, url, method)

	var result interface{}

	switch url {
	case "/odoo/health":
		logger.Infof("[mock brain] Action health ping: returning success")
		result = map[string]string{"status": "ok"}
		m.reportActionResult(dev, action.UUID, result)
		go m.callOdooPing(dev)
		return

	case "/odoo/restart":
		// Ignore restart obox call, return success, do NOT attempt to restart eposproxy
		logger.Infof("[mock brain] Action restart: ignored, returning success")
		result = map[string]string{"status": "restarted"}
		m.reportActionResult(dev, action.UUID, result)
		return

	case "/odoo/disconnect":
		logger.Infof("[mock brain] Action disconnect: returning success")
		result = map[string]string{"status": "disconnected"}
		m.reportActionResult(dev, action.UUID, result)
		return

	case "/odoo/discover_devices":
		logger.Infof("[mock brain] Action discover_devices: fetching printer list")
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

	case "/sos/v1/enable":
		result = "enabled"
		m.reportActionResult(dev, action.UUID, result)
		return

	case "/sos/v1/disable":
		result = "disabled"
		m.reportActionResult(dev, action.UUID, result)
		return

	case "/usb/v1/printer/print":
		logger.Infof("hello")
		result = map[string]string{"status": "ok"}
		m.reportActionResult(dev, action.UUID, result)
		return

	default:
		result = m.dispatchLocalAction(url, method, payload)
		m.reportActionResult(dev, action.UUID, result)
	}
}

// dispatchLocalAction calls the mock device endpoint and returns the result.
func (m *oboxModule) dispatchLocalAction(path, method string, payload interface{}) interface{} {
	base := fmt.Sprintf("http://%s", mockLocalIP)
	fullURL := base + path

	var req *http.Request
	var err error

	switch method {
	case "POST":
		var bodyBytes []byte
		bodyBytes, err = json.Marshal(payload)
		if err != nil {
			logger.Errorf("[mock brain] marshal payload error: %v", err)
			return "ok"
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
		return "ok"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("[mock brain] local action HTTP error url=%s: %v", fullURL, err)
		return "ok"
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
					"local_ip":      mockLocalIP,
					"services":      []string{"odoo", "usb"},
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
