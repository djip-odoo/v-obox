package server

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"time"

	"epos-proxy/logger"
	"epos-proxy/printer"
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
	url, _ := action.Payload["url"].(string)
	method, _ := action.Payload["method"].(string)
	payload := action.Payload["payload"]

	logger.Infof("[mock brain] Executing queue action uuid=%s url=%s method=%s", action.UUID, url, method)

	var result interface{}

	switch url {
	case "/odoo/health":
		logger.Infof("[mock brain] Action health ping: returning success")
		result = map[string]string{"status": "ok"}
		s.reportActionResult(dev, action.UUID, result)
		go s.callOdooPing(dev)
		return

	case "/odoo/restart":
		// Ignore restart obox call, return success, do NOT attempt to restart eposproxy
		logger.Infof("[mock brain] Action restart: ignored, returning success")
		result = map[string]string{"status": "restarted"}
		s.reportActionResult(dev, action.UUID, result)
		return

	case "/odoo/disconnect":
		logger.Infof("[mock brain] Action disconnect: returning success")
		result = map[string]string{"status": "disconnected"}
		s.reportActionResult(dev, action.UUID, result)
		return

	case "/odoo/discover_devices":
		logger.Infof("[mock brain] Action discover_devices: fetching printer list")
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

	case "/sos/v1/enable":
		result = "enabled"
		s.reportActionResult(dev, action.UUID, result)
		return

	case "/sos/v1/disable":
		result = "disabled"
		s.reportActionResult(dev, action.UUID, result)
		return

	case "/usb/v1/printer/print":
		logger.Infof("hello")
		result = map[string]string{"status": "ok"}
		s.reportActionResult(dev, action.UUID, result)
		return

	default:
		result = s.dispatchLocalAction(url, method, payload)
		s.reportActionResult(dev, action.UUID, result)
	}
}

// dispatchLocalAction calls the mock device endpoint and returns the result.
func (s *Server) dispatchLocalAction(path, method string, payload interface{}) interface{} {
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
