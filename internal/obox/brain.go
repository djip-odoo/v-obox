package obox

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"
)

// QueueAction represents an action sent from Odoo to be executed locally.
type QueueAction struct {
	UUID    string                 `json:"uuid"`
	Payload map[string]interface{} `json:"payload"`
}

// DeviceEntry represents a discovered printer in Obox format.
type DeviceEntry struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
	Type       string `json:"type"`
}

// rpcError carries the raw JSON-RPC error body returned by the server.
type rpcError struct{ raw string }

func (e *rpcError) Error() string {
	return "server RPC error: " + e.raw
}

func isDeviceNotFound(err error) bool {
	var rpcErr *rpcError
	if !errors.As(err, &rpcErr) {
		return false
	}
	return strings.Contains(rpcErr.raw, `"code": 404`) ||
		strings.Contains(rpcErr.raw, `"code":404`)
}

func (m *Module) deviceBrain() {
	logger.Infof("[obox brain] Background polling worker started")
	for {
		time.Sleep(5 * time.Second)

		dbURL, token, _ := m.GetCredentials()
		if dbURL == "" || token == "" {
			m.setLiveStatus("disconnected")
			continue
		}

		actions, err := m.fetchNextActions(dbURL, token)
		if err != nil {
			if isDeviceNotFound(err) {
				logger.Warnf("[obox brain] Device not found on server, disconnecting: %v", err)
				m.Disconnect()
				continue
			}
			logger.Infof("[obox brain] fetchNextActions: %v", err)
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
			go m.ExecuteAction(action)
		}
	}
}

func (m *Module) fetchNextActions(dbURL, token string) ([]QueueAction, error) {
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
			"serial_number": m.appID,
			"token":         token,
		},
	})

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(dbURL+"/obox/get_next_actions", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d from /obox/get_next_actions", resp.StatusCode)
	}

	var rpcResp struct {
		Result []QueueAction    `json:"result"`
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

// ExecuteAction processes one queue action and reports the result back to Odoo.
func (m *Module) ExecuteAction(action QueueAction) {
	rawURL, _ := action.Payload["url"].(string)
	method, _ := action.Payload["method"].(string)
	payload := action.Payload["payload"]

	actionPath := rawURL
	if parsed, err := url.Parse(rawURL); err == nil && parsed.Path != "" {
		actionPath = parsed.Path
	}

	logger.Infof("[obox brain] Executing queue action uuid=%s path=%s method=%s", action.UUID, actionPath, method)

	var result interface{}

	switch {
	case actionPath == "/odoo/health":
		logger.Infof("[obox brain] Action health ping: returning success")
		result = map[string]string{"status": "ok"}
		m.reportActionResult(action.UUID, result)
		go m.callOdooPing()
		return

	case actionPath == "/odoo/restart":
		logger.Infof("[obox brain] Action restart: ignored, returning success")
		result = map[string]string{"status": "restarted"}
		m.reportActionResult(action.UUID, result)
		return

	case actionPath == "/odoo/disconnect":
		logger.Infof("[obox brain] Action disconnect: returning success")
		m.Disconnect()
		result = map[string]string{"status": "disconnected"}
		m.reportActionResult(action.UUID, result)
		return

	case actionPath == "/odoo/discover_devices":
		logger.Infof("[obox brain] Action discover_devices: fetching device list")
		devices := m.buildDeviceList()
		devicesJSON, err := json.Marshal(devices)
		if err == nil {
			result = string(devicesJSON)
		} else {
			result = "[]"
		}
		m.reportActionResult(action.UUID, result)
		return

	case actionPath == "/usb/v1/printer/print":
		logger.Infof("[obox brain] Action printer print: executing print simulation")
		result = map[string]string{"status": "ok"}
		m.reportActionResult(action.UUID, result)
		return

	default:
		result = m.dispatchLocalAction(rawURL, method, payload)
		m.reportActionResult(action.UUID, result)
	}
}

func (m *Module) dispatchLocalAction(path, method string, payload interface{}) interface{} {
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
			logger.Errorf("[obox brain] marshal payload error: %v", err)
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
		logger.Errorf("[obox brain] build request error: %v", err)
		return map[string]string{"status": "ok"}
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("[obox brain] local action HTTP error url=%s: %v", fullURL, err)
		return map[string]string{"status": "ok"}
	}
	defer resp.Body.Close()

	var result interface{}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr != nil || result == nil {
		return map[string]string{"status": "ok"}
	}
	logger.Infof("[obox brain] local action result: %v", result)
	return result
}

func (m *Module) reportActionResult(uuid string, result interface{}) {
	dbURL, token, _ := m.GetCredentials()
	if dbURL == "" || token == "" {
		return
	}
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
			"serial_number": m.appID,
			"token":         token,
			"action_uuid":   uuid,
			"result":        result,
		},
	})

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(dbURL+"/obox/action_result", "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Errorf("[obox brain] action_result error for uuid %s: %v", uuid, err)
		return
	}
	defer resp.Body.Close()
	logger.Infof("[obox brain] action_result response status: %d for uuid %s", resp.StatusCode, uuid)
}

func (m *Module) callOdooPing() {
	dbURL, token, _ := m.GetCredentials()
	if dbURL == "" || token == "" {
		return
	}
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
			"serial_number": m.appID,
			"token":         token,
		},
	})

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(dbURL+"/obox/ping", "application/json", bytes.NewReader(body))
	if err != nil {
		logger.Errorf("[obox brain] /obox/ping error: %v", err)
		return
	}
	defer resp.Body.Close()
	logger.Infof("[obox brain] /obox/ping response status: %d", resp.StatusCode)
}

func (m *Module) buildDeviceList() []DeviceEntry {
	var cfg *config.Manager
	if m.cfg != nil {
		cfg = m.cfg
	}
	discovered := printer.DiscoverAllPrinters(cfg)
	devices := make([]DeviceEntry, 0, len(discovered.Available))

	for _, p := range discovered.Available {
		devices = append(devices, DeviceEntry{
			Name:       p.Name,
			Identifier: p.Identifier,
			Type:       "printer",
		})
	}

	logger.Infof("[obox] buildDeviceList: %d devices found", len(devices))
	return devices
}

func (m *Module) callOdooOboxConnect(dbURL, token, dbUUID string) {
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
				"serial_number": m.appID,
				"token":         token,
				"local_ip":      localIP,
				"services":      []string{"usb", "printer"},
			},
		}

		body, err := json.Marshal(payload)
		if err != nil {
			logger.Errorf("[obox] callOdooOboxConnect marshal error: %v", err)
			return
		}

		resp, err := client.Post(endpoint, "application/json", bytes.NewReader(body))
		if err != nil {
			logger.Warnf("[obox] /obox/connect attempt %d connection error: %v", attempt, err)
			continue
		}

		var rpcResp struct {
			Result *json.RawMessage `json:"result"`
			Error  *json.RawMessage `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&rpcResp)
		resp.Body.Close()

		if resp.StatusCode == 200 && rpcResp.Error == nil {
			logger.Infof("[obox] Odoo /obox/connect SUCCESS on attempt %d (paired as %s)", attempt, m.appID)
			m.SetCredentials(dbURL, token, dbUUID)
			if m.cfg != nil {
				_ = m.cfg.SetOdooCredentials(dbURL, token, dbUUID)
			}
			m.setLiveStatus("connected")
			return
		}

		if rpcResp.Error != nil {
			logger.Warnf("[obox] /obox/connect (serial=%s) error from Odoo: %s", m.appID, string(*rpcResp.Error))
		} else {
			logger.Warnf("[obox] /obox/connect (serial=%s) HTTP %d", m.appID, resp.StatusCode)
		}
	}

	logger.Errorf("[obox] Failed to complete /obox/connect after 10 attempts")
}
