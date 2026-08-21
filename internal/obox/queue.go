package obox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"epos-proxy/internal/logger"
)

type QueueAction struct {
	UUID    string        `json:"uuid"`
	Payload ActionPayload `json:"payload"`
}

type ActionPayload struct {
	URL     string `json:"url"`
	Method  string `json:"method"`
	Payload any    `json:"payload"`
}

func (m *Module) oboxQueueHandler(ctx context.Context) {
	logger.Infof("[obox queue] Background polling worker started")
	defer logger.Infof("[obox queue] Background polling worker stopped")

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		dbURL, token := m.GetCredentials()
		if dbURL == "" || token == "" {
			m.setLiveStatus("disconnected")

			return
		}

		actions, err := m.fetchNextActions(dbURL, token)
		if err != nil {
			if isDeviceNotFound(err) {
				logger.Warnf("[obox queue] Device not found on server, disconnecting: %v", err)
				m.Disconnect()
				return
			}

			logger.Infof("[obox queue] fetchNextActions: %v", err)

			last := m.lastContactTime.Load()
			if last == 0 || time.Since(time.UnixMilli(last)) > 10*time.Second {
				m.setLiveStatus("disconnected")
			} else {
				m.setLiveStatus("connecting")
			}

			timer.Reset(5 * time.Second)
			continue
		}

		m.setLiveStatus("connected")
		m.lastContactTime.Store(time.Now().UnixMilli())

		for _, action := range actions {
			m.executeAction(action)
		}

		timer.Reset(5 * time.Second)
	}
}

func (m *Module) fetchNextActions(dbURL, token string) ([]QueueAction, error) {
	resp, err := m.postJSONRPC(dbURL+"/obox/get_next_actions", map[string]string{"serial_number": m.appID, "token": token})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d from /obox/get_next_actions", resp.StatusCode)
	}

	var rpcResp struct {
		Result []QueueAction `json:"result"`
		Error  *rpcError     `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}
	return rpcResp.Result, nil
}

func (m *Module) executeAction(action QueueAction) {
	actionPath := action.Payload.URL
	if parsed, err := url.Parse(actionPath); err == nil && parsed.Path != "" {
		actionPath = parsed.Path
	}

	logger.Infof("[obox queue] Executing queue action uuid=%s path=%s method=%s", action.UUID, actionPath, action.Payload.Method)

	var result interface{}

	switch {
	case actionPath == "/odoo/health":
		logger.Infof("[obox queue] Action health ping: returning success")
		result = map[string]string{"status": "ok"}
		m.reportActionResult(action.UUID, result)
		go m.callOdooPing()
		return

	case actionPath == "/odoo/restart":
		logger.Infof("[obox queue] Action restart requested: not supported on desktop Obox app")
		result = map[string]string{
			"status":  "not_supported",
			"message": "Restart is not supported on Obox app",
		}
		m.reportActionResult(action.UUID, result)
		return

	case actionPath == "/odoo/disconnect":
		logger.Infof("[obox queue] Action disconnect: returning success")
		result = map[string]string{"status": "disconnected"}
		m.reportActionResult(action.UUID, result)
		m.Disconnect()
		return

	case actionPath == "/odoo/discover_devices":
		logger.Infof("[obox queue] Action discover_devices: fetching device list")
		devices := m.buildDeviceList()
		devicesJSON, err := json.Marshal(devices)
		if err == nil {
			result = string(devicesJSON)
		} else {
			result = "[]"
		}
		m.reportActionResult(action.UUID, result)
		return

	case strings.HasPrefix(actionPath, "/sos/v1/"):
		logger.Infof("[obox queue] Action remote debug: not supported on desktop Obox app")
		result = map[string]string{"error": "remote debug not supported on Obox app"}
		m.reportActionResult(action.UUID, result)
		return

	case actionPath == "/usb/v1/printer/print": // TODO: in office printer
		logger.Infof("[obox queue] Action printer print: executing directly")
		result = map[string]string{"error": "printer print not supported on Obox app"}
		m.reportActionResult(action.UUID, result)
		return

	case strings.Contains(actionPath, "/cgi-bin/epos/service.cgi"):
		result, err := m.dispatchLocalAction(m.ctx, action.Payload)
		if err != nil {
			logger.Errorf("[obox queue] local action error uuid=%s path=%s method=%s: %v", action.UUID, actionPath, action.Payload.Method, err)
			result = map[string]string{"error": fmt.Sprintf("local HTTP request failed: %v", err)}
		}
		m.reportActionResult(action.UUID, result)
		return

	default:
		logger.Warnf("[obox queue] Action %s: unsupported on desktop Obox app", actionPath)
		result = map[string]string{"error": "action not supported on Obox app"}
		m.reportActionResult(action.UUID, result)
		return
	}
}
