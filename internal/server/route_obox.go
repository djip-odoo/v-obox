package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"

	"github.com/gofiber/fiber/v3"
)

var obox *oboxModule

// oboxDevice holds the credentials needed to poll Odoo
type oboxDevice struct {
	dbURL  string
	token  string
	dbUuid string
}

type oboxModule struct {
	appId string

	cfg             *config.Manager
	localAddrFn     func() string
	device          atomic.Pointer[oboxDevice]
	liveStatus      atomic.Pointer[string]
	lastContactTime atomic.Int64

	listenersMu sync.RWMutex
	listeners   []StatusListener
}

// StatusListener represents a callback function for Odoo status changes.
type StatusListener func()

func (s *Server) GetWebsocketStatus() string {
	if obox != nil {
		return obox.getWebsocketStatus()
	}
	return "disconnected"
}

func (s *Server) GetOdooDbURL() string {
	return obox.getDbURL()
}

func (m *oboxModule) getDbURL() string {
	if dev := m.device.Load(); dev != nil {
		return dev.dbURL
	}
	return m.cfg.GetOdooDbURL()
}

// DisconnectOdoo clears in-memory device credentials and removes stored config.
func (s *Server) DisconnectOdoo() {
	if obox != nil {
		obox.disconnect()
	}
}

// OnStatusChange registers a callback to be called whenever OdooStatus changes.
func (s *Server) OnStatusChange(listener StatusListener) {
	if obox != nil {
		obox.onStatusChange(listener)
	}
}

func (m *oboxModule) onStatusChange(listener StatusListener) {
	m.listenersMu.Lock()
	defer m.listenersMu.Unlock()
	m.listeners = append(m.listeners, listener)
}

func (m *oboxModule) notifyStatusChange() {
	m.listenersMu.RLock()
	listeners := make([]StatusListener, len(m.listeners))
	copy(listeners, m.listeners)
	m.listenersMu.RUnlock()

	for _, l := range listeners {
		l()
	}
}

func (m *oboxModule) setLiveStatus(st string) {
	prev := m.liveStatus.Load()
	changed := prev == nil || *prev != st
	m.liveStatus.Store(&st)
	if changed {
		m.notifyStatusChange()
	}
}

func init() {
	RegisterRoute(func(s *Server, cfg *config.Manager) {
		obox = newOboxModule(s.app, s.mgr, cfg, s.LocalAddr)
	})
}

func newOboxModule(app *fiber.App, mgr *printer.Manager, cfg *config.Manager, localAddrFn func() string) *oboxModule {
	m := &oboxModule{cfg: cfg, localAddrFn: localAddrFn}
	initialStatus := "disconnected"
	if cfg.HasOdooCredentials() {
		odooCfg := cfg.GetOdooConfig()
		m.device.Store(&oboxDevice{
			dbURL:  odooCfg.DbURL,
			token:  odooCfg.Token,
			dbUuid: odooCfg.DbUUID,
		})
		initialStatus = "connecting"
		logger.Infof("[mock] Restored Odoo credentials from storage: db=%s dbuuid=%s", odooCfg.DbURL, odooCfg.DbUUID)
	}
	m.setLiveStatus(initialStatus)
	m.registerRoutes(app, mgr)
	m.appId = cfg.GetAppID()
	go m.deviceBrain()
	return m
}

func (m *oboxModule) getWebsocketStatus() string {
	if dev := m.device.Load(); dev != nil && dev.dbURL != "" {
		if ptr := m.liveStatus.Load(); ptr != nil && *ptr != "" {
			return *ptr
		}
		return "connecting"
	}
	if m.cfg != nil && m.cfg.HasOdooCredentials() {
		return "connecting"
	}
	return "disconnected"
}

func (m *oboxModule) disconnect() {
	logger.Infof("[mock] Obox disconnect triggered")
	m.device.Store(nil)
	m.setLiveStatus("disconnected")
	if m.cfg != nil {
		if err := m.cfg.ClearOdooConfig(); err != nil {
			logger.Warnf("[mock] Failed to clear Odoo credentials from storage: %v", err)
		}
	}
}

func (m *oboxModule) registerRoutes(app *fiber.App, mgr *printer.Manager) {
	app.Get("/odoo/", func(ctx fiber.Ctx) error {
		logger.Debug("[mock] Obox LAN health check /odoo/")
		dev := m.device.Load()
		if dev != nil {
			serial := ""
			if m.cfg != nil {
				serial = m.cfg.GetAppID()
			}
			return ctx.JSON(map[string]interface{}{
				"status": "configured",
				"data": map[string]string{
					"serial": serial,
					"db_url": dev.dbURL,
				},
			})
		}
		return ctx.JSON(map[string]interface{}{
			"status": "configured",
			"data":   nil,
		})
	})

	app.Get("/odoo/health", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox /odoo/health ping")
		dev := m.device.Load()
		if dev != nil {
			go m.callOdooPing(dev)
		}
		return ctx.JSON(map[string]string{"status": "ok"})
	})

	// Ignore restart obox call, return success, do NOT attempt to restart eposproxy
	app.Get("/odoo/restart", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox restart — ignored, returning success")
		return ctx.JSON(map[string]string{"status": "restarted"})
	})

	app.Get("/odoo/disconnect", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox disconnect — clearing device credentials")
		m.device.Store(nil)
		m.setLiveStatus("disconnected")
		if m.cfg != nil {
			if err := m.cfg.ClearOdooConfig(); err != nil {
				logger.Warnf("[mock] Failed to clear Odoo credentials from storage: %v", err)
			}
		}
		return ctx.JSON(map[string]string{"status": "disconnected"})
	})

	app.Get("/odoo/discover_devices", func(ctx fiber.Ctx) error {
		logger.Infof("[mock] Obox discover_devices")
		devices := m.buildDeviceList()
		return ctx.JSON(devices)
	})

	app.Get("/odoo/connect", func(ctx fiber.Ctx) error {
		dbURL := ctx.Query("db_url")
		token := ctx.Query("token")
		dbUUID := ctx.Query("db_uuid")

		logger.Infof("[mock] Obox offline connect received: db_url=%s, token=%s, db_uuid=%s", dbURL, token, dbUUID)
		if dbURL != "" && token != "" {
			m.device.Store(&oboxDevice{dbURL: dbURL, token: token, dbUuid: dbUUID})
			m.setLiveStatus("connecting")
			if m.cfg != nil {
				if err := m.cfg.SetOdooCredentials(dbURL, token, dbUUID); err != nil {
					logger.Warnf("[mock] Failed to save Odoo credentials to storage: %v", err)
				}
			}
			go m.callOdooOboxConnect(dbURL, token, dbUUID)
		}
		return ctx.SendStatus(fiber.StatusOK)
	})
}

func (m *oboxModule) deviceBrain() {
	logger.Infof("[mock brain] Background polling worker started")
	for {
		time.Sleep(5 * time.Second)

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
			"serial_number": m.appId,
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

	case actionPath == "/usb/v1/printer/print":
		logger.Infof("[mock brain] Action printer print: executing print simulation")
		result = map[string]string{"status": "ok"}
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

func (m *oboxModule) dispatchLocalAction(path, method string, payload interface{}) interface{} {
	localAddr := m.localAddrFn()
	base := fmt.Sprintf("http://%s", localAddr)
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
			"serial_number": m.appId,
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
			"serial_number": m.appId,
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
	var cfg *config.Manager
	if m.cfg != nil {
		cfg = m.cfg
	}
	discovered := printer.DiscoverAllPrinters(cfg)
	devices := make([]oboxDeviceEntry, 0, len(discovered.Available))

	for _, p := range discovered.Available {
		devices = append(devices, oboxDeviceEntry{
			Name:       p.Name,
			Identifier: p.Identifier,
			Type:       "printer",
		})
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

		localIP := ""
		if m.localAddrFn != nil {
			localIP = m.localAddrFn()
		}

		payload := rpcPayload{
			JSONRPC: "2.0",
			Method:  "call",
			ID:      1,
			Params: map[string]interface{}{
				"serial_number": m.appId,
				"token":         token,
				"local_ip":      localIP,
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
			logger.Infof("[mock] Odoo /obox/connect SUCCESS on attempt %d (paired as %s)", attempt, m.appId)
			m.device.Store(&oboxDevice{
				dbURL:  dbURL,
				token:  token,
				dbUuid: dbUuid,
			})
			if m.cfg != nil {
				_ = m.cfg.SetOdooCredentials(dbURL, token, dbUuid)
			}
			m.setLiveStatus("connected")
			return
		}

		if rpcResp.Error != nil {
			logger.Warnf("[mock] /obox/connect (serial=%s) error from Odoo: %s", m.appId, string(*rpcResp.Error))
		} else {
			logger.Warnf("[mock] /obox/connect (serial=%s) HTTP %d", m.appId, resp.StatusCode)
		}
	}

	logger.Errorf("[mock] Failed to complete /obox/connect after 10 attempts")
}
