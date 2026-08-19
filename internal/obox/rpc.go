package obox

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"epos-proxy/internal/logger"
)

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	} `json:"data"`
}

func (e *rpcError) Error() string {
	if e.Data.Name != "" {
		return fmt.Sprintf("server RPC error %d (%s): %s", e.Code, e.Data.Name, e.Message)
	}
	return fmt.Sprintf("server RPC error %d: %s", e.Code, e.Message)
}

func isDeviceNotFound(err error) bool {
	var rpcErr *rpcError
	if !errors.As(err, &rpcErr) {
		return false
	}
	return rpcErr.Code == http.StatusNotFound
}

func (m *Module) reportActionResult(uuid string, result interface{}) {
	dbURL, token := m.GetCredentials()
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
		logger.Errorf("[obox queue] action_result error for uuid %s: %v", uuid, err)
		return
	}
	defer resp.Body.Close()
	logger.Infof("[obox queue] action_result response status: %d for uuid %s", resp.StatusCode, uuid)
}

func (m *Module) callOdooPing() {
	dbURL, token := m.GetCredentials()
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
		logger.Errorf("[obox queue] /obox/ping error: %v", err)
		return
	}
	defer resp.Body.Close()
	logger.Infof("[obox queue] /obox/ping response status: %d", resp.StatusCode)
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

		payload := rpcPayload{
			JSONRPC: "2.0",
			Method:  "call",
			ID:      1,
			Params: map[string]interface{}{
				"serial_number": m.appID,
				"token":         token,
				"local_ip":      m.localAddrFn(),
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
