package obox

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"epos-proxy/internal/logger"

	"github.com/gorilla/websocket"
)

type busSubscribePayload struct {
	EventName string           `json:"event_name"`
	Data      busSubscribeData `json:"data"`
}

type busSubscribeData struct {
	Channels []string `json:"channels"`
	Last     int      `json:"last"`
}

func buildWebsocketURL(dbURL string) (string, error) {
	raw := strings.TrimSpace(dbURL)
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	case "wss", "ws":
		// already websocket scheme
	default:
		parsed.Scheme = "ws"
	}

	parsed.Path = "/websocket"
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func (m *Module) oboxWebsocketHandler() {
	logger.Infof("[obox ws] Background websocket worker started")
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-m.ctx.Done():
			logger.Infof("[obox ws] Background websocket worker stopped")
			return
		default:
		}

		dbURL, token := m.GetCredentials()
		if dbURL == "" || token == "" {
			select {
			case <-m.ctx.Done():
				return
			case <-time.After(1 * time.Second):
				continue
			}
		}

		err := m.runWebsocketSession(dbURL, token)
		if err != nil {
			logger.Debugf("[obox ws] Session ended: %v", err)
		}

		select {
		case <-m.ctx.Done():
			return
		case <-time.After(backoff):
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

func (m *Module) runWebsocketSession(dbURL, token string) error {
	wsURL, err := buildWebsocketURL(dbURL)
	if err != nil {
		return fmt.Errorf("invalid websocket URL from dbURL %s: %w", dbURL, err)
	}

	logger.Infof("[obox ws] Connecting to Odoo websocket: %s", wsURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
	}

	header := http.Header{}
	conn, resp, err := dialer.DialContext(m.ctx, wsURL, header)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return fmt.Errorf("dial websocket error: %w", err)
	}
	defer conn.Close()

	// Subscribe to obox_<token> channel
	channel := "obox_" + token
	subPayload := busSubscribePayload{
		EventName: "subscribe",
		Data: busSubscribeData{
			Channels: []string{channel},
			Last:     0,
		},
	}

	if err := conn.WriteJSON(subPayload); err != nil {
		return fmt.Errorf("failed to send subscription to channel %s: %w", channel, err)
	}

	logger.Infof("[obox ws] Subscribed to websocket channel: %s", channel)
	m.lastContactTime.Store(time.Now().UnixMilli())
	m.setLiveStatus("connected")

	// Fetch any pending actions right away on connection
	m.TriggerFetch()

	// Ensure connection is closed when context is done
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-m.ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		m.lastContactTime.Store(time.Now().UnixMilli())
		m.setLiveStatus("connected")

		msgStr := string(message)
		logger.Debugf("[obox ws] Received websocket message: %s", msgStr)

		if m.isActionNotification(message, channel) {
			logger.Infof("[obox ws] ACTION notification received on channel %s, triggering immediate fetch", channel)
			m.TriggerFetch()
		}
	}
}

// isActionNotification checks whether the incoming frame represents an ACTION notification
// or targets the obox channel.
func (m *Module) isActionNotification(raw []byte, targetChannel string) bool {
	str := string(raw)

	// Quick string checks for common patterns
	if strings.Contains(str, `"ACTION"`) || strings.Contains(str, targetChannel) {
		return true
	}

	// Structured JSON check for Odoo bus notifications
	// Array format: [ { "id": 1, "message": { "type": "ACTION", ... } } ]
	var arrayNotifications []struct {
		ID      interface{} `json:"id"`
		Channel interface{} `json:"channel"`
		Message interface{} `json:"message"`
	}
	if err := json.Unmarshal(raw, &arrayNotifications); err == nil && len(arrayNotifications) > 0 {
		for _, n := range arrayNotifications {
			if msgMap, ok := n.Message.(map[string]interface{}); ok {
				if t, ok := msgMap["type"].(string); ok && t == "ACTION" {
					return true
				}
			}
			if msgStr, ok := n.Message.(string); ok && msgStr == "ACTION" {
				return true
			}
		}
	}

	return false
}
