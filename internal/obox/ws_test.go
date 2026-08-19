package obox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"epos-proxy/internal/config"
	"epos-proxy/internal/testutil"

	"github.com/gorilla/websocket"
)

func TestBuildWebsocketURL(t *testing.T) {
	tests := []struct {
		name      string
		dbURL     string
		expected  string
		expectErr bool
	}{
		{
			name:     "http url",
			dbURL:    "http://127.0.0.1:8069",
			expected: "ws://127.0.0.1:8069/websocket",
		},
		{
			name:     "http url with trailing slash",
			dbURL:    "http://127.0.0.1:8069/",
			expected: "ws://127.0.0.1:8069/websocket",
		},
		{
			name:     "https url",
			dbURL:    "https://my-database.odoo.com",
			expected: "wss://my-database.odoo.com/websocket",
		},
		{
			name:     "ws url already",
			dbURL:    "ws://localhost:8069",
			expected: "ws://localhost:8069/websocket",
		},
		{
			name:     "wss url already",
			dbURL:    "wss://localhost:8069",
			expected: "wss://localhost:8069/websocket",
		},
		{
			name:     "no scheme defaults to ws",
			dbURL:    "localhost:8069",
			expected: "ws://localhost:8069/websocket",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildWebsocketURL(tc.dbURL)
			if tc.expectErr {
				testutil.ExpectedError(t, err)
			} else {
				testutil.ExpectedNoError(t, err)
				testutil.ExpectedEqual(t, got, tc.expected)
			}
		})
	}
}

func TestIsActionNotification(t *testing.T) {
	m := &Module{}
	targetChannel := "obox_token123"

	// 1. Raw string contains "ACTION"
	testutil.ExpectedTrue(t, m.isActionNotification([]byte(`["ACTION"]`), targetChannel))

	// 2. Raw string contains target channel
	testutil.ExpectedTrue(t, m.isActionNotification([]byte(`{"channel": "obox_token123"}`), targetChannel))

	// 3. Array of notifications with type: "ACTION"
	arrayJSON := []byte(`[{"id": 10, "channel": "some_chan", "message": {"type": "ACTION", "payload": ""}}]`)
	testutil.ExpectedTrue(t, m.isActionNotification(arrayJSON, targetChannel))

	// 4. Array with string message "ACTION"
	arrayStrJSON := []byte(`[{"id": 10, "channel": "some_chan", "message": "ACTION"}]`)
	testutil.ExpectedTrue(t, m.isActionNotification(arrayStrJSON, targetChannel))

	// 5. Unrelated message
	unrelatedJSON := []byte(`[{"id": 10, "channel": "other_chan", "message": {"type": "CHAT", "payload": "hello"}}]`)
	testutil.ExpectedFalse(t, m.isActionNotification(unrelatedJSON, targetChannel))
}

func TestWebsocketSession_SubscribeAndActionTrigger(t *testing.T) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	subscribedChan := make(chan string, 1)
	triggerReceived := make(chan struct{}, 1)

	mockWSServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/websocket") {
			http.NotFound(w, r)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Read subscription message
		var sub busSubscribePayload
		if err := conn.ReadJSON(&sub); err != nil {
			return
		}
		if len(sub.Data.Channels) > 0 {
			subscribedChan <- sub.Data.Channels[0]
		}

		// Send an ACTION notification frame
		actionFrame := []map[string]interface{}{
			{
				"id": 1,
				"message": map[string]interface{}{
					"type": "ACTION",
				},
			},
		}
		_ = conn.WriteJSON(actionFrame)

		// Keep connection open until client disconnects
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer mockWSServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	triggerChan := make(chan struct{}, 10)
	m := &Module{
		cfg:         &config.Manager{},
		appID:       "ODOOAPP_TEST_123",
		triggerChan: triggerChan,
		ctx:         ctx,
		cancel:      cancel,
	}

	testToken := "tok_abc_456"
	m.SetCredentials(mockWSServer.URL, testToken, "uuid_123")

	// Drain initial TriggerFetch caused by SetCredentials
	select {
	case <-triggerChan:
	default:
	}

	go func() {
		_ = m.runWebsocketSession(mockWSServer.URL, testToken)
	}()

	// 1. Verify subscription was sent
	select {
	case subChannel := <-subscribedChan:
		testutil.ExpectedEqual(t, subChannel, "obox_"+testToken)
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for websocket subscribe message")
	}

	// 2. Verify ACTION notification triggered fetch
	select {
	case <-triggerChan:
		triggerReceived <- struct{}{}
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for ACTION notification to trigger fetch")
	}

	testutil.ExpectedEqual(t, m.GetWebsocketStatus(), "connected")

	// 3. Clean shutdown
	cancel()
}
