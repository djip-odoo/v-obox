package obox

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"epos-proxy/internal/config"
	"epos-proxy/internal/testutil"

	"github.com/gofiber/fiber/v3"
)

func createTestModule(t *testing.T) (*Module, *fiber.App) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)
	app := fiber.New()

	m := NewModule(cfg, 4545)
	t.Cleanup(m.Stop)
	app.Get("/odoo/", m.HandleDiscovery)
	app.Get("/odoo/connect", m.HandleConnect)
	app.Get("/odoo/disconnect", m.HandleDisconnect)
	return m, app
}

func TestObox_CredentialsAndConnection(t *testing.T) {
	m, _ := createTestModule(t)
	testutil.ExpectedEqual(t, m.GetWebsocketStatus(), "disconnected")

	m.SetCredentials("http://127.0.0.1:8069", "token-xyz", "db-uuid-1")

	dbURL, tok := m.GetCredentials()
	testutil.ExpectedEqual(t, dbURL, "http://127.0.0.1:8069")
	testutil.ExpectedEqual(t, tok, "token-xyz")
	testutil.ExpectedEqual(t, m.GetDbURL(), "http://127.0.0.1:8069")

	m.ClearCredentials()
	testutil.ExpectedEqual(t, m.GetDbURL(), "")
}

func TestObox_StatusChangeListener(t *testing.T) {
	m, _ := createTestModule(t)

	called := false
	m.OnStatusChange(func() {
		called = true
	})

	m.setLiveStatus("connected")
	testutil.ExpectedTrue(t, called)
}

func TestObox_Routes(t *testing.T) {
	// Use a real mock server so the background queue handler and callOdooOboxConnect
	// goroutines have a reachable target and don't hang indefinitely.
	mockOdoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": []interface{}{}})
	}))
	defer mockOdoo.Close()

	m, app := createTestModule(t)

	// 1. Initial /odoo/ (not configured)
	req := httptest.NewRequest("GET", "/odoo/", nil)
	resp, err := app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	// 2. /odoo/connect — use the mock server URL so background goroutines
	// (queue handler, callOdooOboxConnect) reach a live endpoint rather than
	// blocking forever on an unreachable host.
	connectURL := "/odoo/connect?db_url=" + mockOdoo.URL + "&token=test-tok&db_uuid=test-uuid"
	req = httptest.NewRequest("GET", connectURL, nil)
	resp, err = app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	// 3. /odoo/ configured discovery
	req = httptest.NewRequest("GET", "/odoo/", nil)
	resp, err = app.Test(req)
	testutil.ExpectedNoError(t, err)
	var discResp struct {
		Status string            `json:"status"`
		Data   map[string]string `json:"data"`
	}
	err = json.NewDecoder(resp.Body).Decode(&discResp)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, discResp.Status, "configured")
	testutil.ExpectedEqual(t, discResp.Data["db_url"], mockOdoo.URL)
	resp.Body.Close()

	// 4. Disconnect — also stops the queue handler goroutine before the test exits.
	reqDisc := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	respDisc, err := app.Test(reqDisc)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, respDisc.StatusCode, http.StatusOK)
	respDisc.Body.Close()

	// Ensure m.Stop cancels any remaining goroutines (callOdooOboxConnect retries).
	m.Stop()
}

func TestObox_executeAction(t *testing.T) {
	actionReported := make(chan string, 10)
	lastResult := make(chan interface{}, 10)

	mockOdoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				ActionUUID string      `json:"action_uuid"`
				Result     interface{} `json:"result"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Params.ActionUUID != "" {
			actionReported <- req.Params.ActionUUID
			lastResult <- req.Params.Result
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": "ok"})
	}))
	defer mockOdoo.Close()

	mockBox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/odoo/health":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case "/odoo/discover_devices":
			_ = json.NewEncoder(w).Encode([]map[string]string{{"name": "p1", "type": "printer"}})
		case "/odoo/restart":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "not_supported"})
		case "/odoo/disconnect":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "disconnected"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockBox.Close()

	boxPort := mockBox.Listener.Addr().(*net.TCPAddr).Port
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)
	m := NewModule(cfg, boxPort)
	t.Cleanup(m.Stop)
	m.SetCredentials(mockOdoo.URL, "tok", "uuid")

	// 1. Health ping action
	actionHealth := QueueAction{
		UUID: "action-health",
		Payload: ActionPayload{
			URL:    "/odoo/health",
			Method: "GET",
		},
	}
	m.executeAction(actionHealth)
	testutil.ExpectedEqual(t, <-actionReported, "action-health")
	resHealth := <-lastResult
	var healthMap map[string]interface{}
	testutil.ExpectedNoError(t, json.Unmarshal([]byte(resHealth.(string)), &healthMap))
	testutil.ExpectedEqual(t, healthMap["status"], "ok")

	// 2. Discover devices action
	actionDiscover := QueueAction{
		UUID: "action-discover",
		Payload: ActionPayload{
			URL:    "/odoo/discover_devices",
			Method: "GET",
		},
	}
	m.executeAction(actionDiscover)
	testutil.ExpectedEqual(t, <-actionReported, "action-discover")
	resDiscover := <-lastResult
	var devList []map[string]interface{}
	testutil.ExpectedNoError(t, json.Unmarshal([]byte(resDiscover.(string)), &devList))
	testutil.ExpectedLen(t, devList, 1)

	// 3. Restart action (not supported)
	actionRestart := QueueAction{
		UUID: "action-restart",
		Payload: ActionPayload{
			URL:    "/odoo/restart",
			Method: "GET",
		},
	}
	m.executeAction(actionRestart)
	testutil.ExpectedEqual(t, <-actionReported, "action-restart")
	resRestart := <-lastResult
	var restartMap map[string]interface{}
	testutil.ExpectedNoError(t, json.Unmarshal([]byte(resRestart.(string)), &restartMap))
	testutil.ExpectedEqual(t, restartMap["status"], "not_supported")

	// 4. Remote debug action (404 not found locally -> returns false string)
	actionDebug := QueueAction{
		UUID: "action-debug",
		Payload: ActionPayload{
			URL:    "/sos/v1/enable?token=123",
			Method: "GET",
		},
	}
	m.executeAction(actionDebug)
	testutil.ExpectedEqual(t, <-actionReported, "action-debug")
	resDebug := <-lastResult
	testutil.ExpectedEqual(t, resDebug, "false")

	// 5. Disconnect action
	actionDisconnect := QueueAction{
		UUID: "action-disc",
		Payload: ActionPayload{
			URL:    "/odoo/disconnect",
			Method: "GET",
		},
	}
	m.executeAction(actionDisconnect)
	testutil.ExpectedEqual(t, <-actionReported, "action-disc")
}

func TestObox_FetchNextActions(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				SerialNumber string `json:"serial_number"`
				Token        string `json:"token"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.Params.Token == "valid-token" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": []QueueAction{
					{
						UUID: "action-uuid-101",
						Payload: ActionPayload{
							URL:    "/odoo/health",
							Method: "GET",
						},
					},
				},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    404,
					"message": "Device not found",
				},
			})
		}
	}))
	defer mockServer.Close()

	m, _ := createTestModule(t)

	// 1. Success case
	actions, err := m.fetchNextActions(mockServer.URL, "valid-token")
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedLen(t, actions, 1)
	testutil.ExpectedEqual(t, actions[0].UUID, "action-uuid-101")

	// 2. Error case (JSON-RPC 404)
	_, err = m.fetchNextActions(mockServer.URL, "invalid-token")
	testutil.ExpectedError(t, err)
	testutil.ExpectedTrue(t, isDeviceNotFound(err))

	// 3. Raw HTTP 404 (non-RPC envelope)
	raw404Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer raw404Server.Close()

	_, err = m.fetchNextActions(raw404Server.URL, "any-token")
	testutil.ExpectedError(t, err)
	testutil.ExpectedTrue(t, isDeviceNotFound(err))
}

func TestObox_CallOdooPing(t *testing.T) {
	pingReceived := make(chan bool, 1)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/obox/ping" {
			pingReceived <- true
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": "ok"})
	}))
	defer mockServer.Close()

	m, _ := createTestModule(t)
	m.SetCredentials(mockServer.URL, "valid-tok", "uuid")

	m.callOdooPing()

	select {
	case <-pingReceived:
		// success
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for ping to be received")
	}
}

func TestObox_BuildDeviceList(t *testing.T) {
	m, _ := createTestModule(t)
	devices := m.buildDeviceList()

	// Should produce a valid list of devices where all entries have type == "printer"
	for _, dev := range devices {
		testutil.ExpectedEqual(t, dev["type"], "printer")
		testutil.ExpectedNotEqual(t, dev["name"], "")
		testutil.ExpectedNotEqual(t, dev["identifier"], "")
	}
}

func TestObox_DispatchLocalAction(t *testing.T) {
	var receivedMethod string
	var receivedBody map[string]interface{}

	mockLocal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		if r.Method == "POST" {
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "dispatched_ok"})
	}))
	defer mockLocal.Close()

	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)

	mockPort := mockLocal.Listener.Addr().(*net.TCPAddr).Port
	m := NewModule(cfg, mockPort)

	// 1. POST dispatch
	resPost, _ := m.dispatchLocalAction(m.ctx, ActionPayload{
		URL:     "/test-post",
		Method:  "POST",
		Payload: json.RawMessage(`{"test_key":"test_val"}`),
	})
	testutil.ExpectedEqual(t, receivedMethod, "POST")
	testutil.ExpectedEqual(t, receivedBody["test_key"], "test_val")
	resMap, ok := resPost.(map[string]interface{})
	testutil.ExpectedTrue(t, ok)
	testutil.ExpectedEqual(t, resMap["result"], "dispatched_ok")

	// 2. GET dispatch
	resGet, _ := m.dispatchLocalAction(m.ctx, ActionPayload{URL: "/test-get", Method: "GET"})
	testutil.ExpectedEqual(t, receivedMethod, "GET")
	resMapGet, ok := resGet.(map[string]interface{})
	testutil.ExpectedTrue(t, ok)
	testutil.ExpectedEqual(t, resMapGet["result"], "dispatched_ok")
}

func TestObox_CallOdooOboxConnect(t *testing.T) {
	connectReceived := make(chan bool, 1)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/obox/connect":
			var req struct {
				Params struct {
					SerialNumber string   `json:"serial_number"`
					Token        string   `json:"token"`
					LocalIP      string   `json:"local_ip"`
					Services     []string `json:"services"`
				} `json:"params"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Params.Token == "conn-tok" {
				connectReceived <- true
				raw := json.RawMessage(`{"status": "paired"}`)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": &raw})
				return
			}
		case "/obox/get_next_actions":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": []interface{}{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"code": 404, "message": "not found"},
		})
	}))
	defer mockServer.Close()

	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)

	m := NewModule(cfg, 4545)
	// Register Stop so that if the test times out, the ctx cancellation stops
	// any in-flight callOdooOboxConnect retry loop immediately.
	t.Cleanup(m.Stop)
	m.SetCredentials(mockServer.URL, "conn-tok", "conn-uuid")

	// callOdooOboxConnect succeeds on first attempt (mock returns 200 + result).
	// Run in a goroutine so we can apply a test-level timeout via select.
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.callOdooOboxConnect(mockServer.URL, "conn-tok", "conn-uuid")
	}()

	select {
	case <-connectReceived:
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for callOdooOboxConnect to finish after handshake")
		}
		dbURL, tok := m.GetCredentials()
		testutil.ExpectedEqual(t, dbURL, mockServer.URL)
		testutil.ExpectedEqual(t, tok, "conn-tok")
		testutil.ExpectedEqual(t, m.GetWebsocketStatus(), "connected")
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for /obox/connect handshake")
	}
}

func TestObox_IsDeviceNotFound(t *testing.T) {
	// 1. Non-rpcError
	testutil.ExpectedFalse(t, isDeviceNotFound(errors.New("regular network error")))
	testutil.ExpectedFalse(t, isDeviceNotFound(nil))

	// 2. rpcError with 404 code
	err404 := &rpcError{Code: 404, Message: "404: Not Found"}
	testutil.ExpectedTrue(t, isDeviceNotFound(err404))
	testutil.ExpectedContains(t, err404.Error(), "404")

	// 3. rpcError with werkzeug NotFound exception name
	errWerkzeug := &rpcError{
		Code:    200,
		Message: "Odoo Error",
		Data: struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		}{
			Name:    "werkzeug.exceptions.NotFound",
			Message: "404 Not Found",
		},
	}
	testutil.ExpectedTrue(t, isDeviceNotFound(errWerkzeug))
	testutil.ExpectedContains(t, errWerkzeug.Error(), "werkzeug.exceptions.NotFound")

	// 4. Other RPC error (e.g. 500 Internal Server Error or AccessDenied)
	err500 := &rpcError{Code: 500, Message: "Internal Server Error"}
	testutil.ExpectedFalse(t, isDeviceNotFound(err500))
}

func TestObox_LANContact(t *testing.T) {
	m, _ := createTestModule(t)

	// Initially inactive
	testutil.ExpectedEqual(t, m.GetLANStatus(), "disconnected")

	statusChanged := false
	m.OnStatusChange(func() {
		statusChanged = true
	})

	// Record LAN contact
	m.RecordLANContact()
	testutil.ExpectedEqual(t, m.GetLANStatus(), "connected")
	testutil.ExpectedTrue(t, statusChanged)

	// Second contact when already active should not trigger notification
	statusChanged = false
	m.RecordLANContact()
	testutil.ExpectedEqual(t, m.GetLANStatus(), "connected")
	testutil.ExpectedFalse(t, statusChanged)

	// Disconnect clears LAN contact
	m.Disconnect()
	testutil.ExpectedEqual(t, m.GetLANStatus(), "disconnected")
}
