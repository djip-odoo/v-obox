package obox

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"epos-proxy/internal/config"
	"epos-proxy/internal/printer"
	"epos-proxy/internal/testutil"

	"github.com/gofiber/fiber/v3"
)

func createTestModule(t *testing.T) (*Module, *fiber.App) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)
	app := fiber.New()

	m := Manager(cfg, printer.NewManager(), func() string { return "127.0.0.1:4545" })
	t.Cleanup(m.Stop)
	app.Get("/odoo/", m.HandleDiscovery)
	app.Get("/odoo/connect", m.HandleConnect)
	app.Get("/odoo/disconnect", m.HandleDisconnect)
	return m, app
}

func TestObox_CredentialsAndConnection(t *testing.T) {
	m, _ := createTestModule(t)

	testutil.ExpectedFalse(t, m.IsConnected())
	testutil.ExpectedEqual(t, m.GetWebsocketStatus(), "disconnected")

	m.SetCredentials("http://127.0.0.1:8069", "token-xyz", "db-uuid-1")
	testutil.ExpectedTrue(t, m.IsConnected())

	dbURL, tok := m.GetCredentials()
	testutil.ExpectedEqual(t, dbURL, "http://127.0.0.1:8069")
	testutil.ExpectedEqual(t, tok, "token-xyz")
	testutil.ExpectedEqual(t, m.GetDbURL(), "http://127.0.0.1:8069")

	m.ClearCredentials()
	testutil.ExpectedFalse(t, m.IsConnected())
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
	m, app := createTestModule(t)

	// 1. Initial /odoo/ (not configured)
	req := httptest.NewRequest("GET", "/odoo/", nil)
	resp, err := app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	// 2. /odoo/connect (offline connect)
	req = httptest.NewRequest("GET", "/odoo/connect?db_url=http://127.0.0.1:8069&token=test-tok&db_uuid=test-uuid", nil)
	resp, err = app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	testutil.ExpectedTrue(t, m.IsConnected())

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
	testutil.ExpectedEqual(t, discResp.Data["db_url"], "http://127.0.0.1:8069")
	resp.Body.Close()

	// 4. In-memory / HTTP Disconnect
	reqDisc := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	respDisc, err := app.Test(reqDisc)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, respDisc.StatusCode, http.StatusOK)
	respDisc.Body.Close()

	testutil.ExpectedFalse(t, m.IsConnected())
}

func TestObox_ExecuteAction(t *testing.T) {
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

	m, _ := createTestModule(t)
	m.SetCredentials(mockOdoo.URL, "tok", "uuid")

	// 1. Health ping action
	actionHealth := QueueAction{
		UUID: "action-health",
		Payload: map[string]interface{}{
			"url":    "/odoo/health",
			"method": "GET",
		},
	}
	m.ExecuteAction(actionHealth)
	testutil.ExpectedEqual(t, <-actionReported, "action-health")
	resHealth := <-lastResult
	healthMap, ok := resHealth.(map[string]interface{})
	testutil.ExpectedTrue(t, ok)
	testutil.ExpectedEqual(t, healthMap["status"], "ok")

	// 2. Discover devices action
	actionDiscover := QueueAction{
		UUID: "action-discover",
		Payload: map[string]interface{}{
			"url":    "/odoo/discover_devices",
			"method": "GET",
		},
	}
	m.ExecuteAction(actionDiscover)
	testutil.ExpectedEqual(t, <-actionReported, "action-discover")
	resDiscover := <-lastResult
	resDiscoverStr, ok := resDiscover.(string)
	testutil.ExpectedTrue(t, ok)
	var devList []map[string]interface{}
	err := json.Unmarshal([]byte(resDiscoverStr), &devList)
	testutil.ExpectedNoError(t, err)

	// 3. Restart action (not supported)
	actionRestart := QueueAction{
		UUID: "action-restart",
		Payload: map[string]interface{}{
			"url":    "/odoo/restart",
			"method": "GET",
		},
	}
	m.ExecuteAction(actionRestart)
	testutil.ExpectedEqual(t, <-actionReported, "action-restart")
	resRestart := <-lastResult
	restartMap, ok := resRestart.(map[string]interface{})
	testutil.ExpectedTrue(t, ok)
	testutil.ExpectedEqual(t, restartMap["status"], "not_supported")

	// 4. Remote debug action (not supported)
	actionDebug := QueueAction{
		UUID: "action-debug",
		Payload: map[string]interface{}{
			"url":    "/sos/v1/enable?token=123",
			"method": "GET",
		},
	}
	m.ExecuteAction(actionDebug)
	testutil.ExpectedEqual(t, <-actionReported, "action-debug")
	resDebug := <-lastResult
	debugMap, ok := resDebug.(map[string]interface{})
	testutil.ExpectedTrue(t, ok)
	testutil.ExpectedNotEqual(t, debugMap["error"], "")

	// 5. Disconnect action
	actionDisconnect := QueueAction{
		UUID: "action-disc",
		Payload: map[string]interface{}{
			"url":    "/odoo/disconnect",
			"method": "GET",
		},
	}
	m.ExecuteAction(actionDisconnect)
	testutil.ExpectedEqual(t, <-actionReported, "action-disc")
	testutil.ExpectedFalse(t, m.IsConnected())
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
						Payload: map[string]interface{}{
							"url":    "/odoo/health",
							"method": "GET",
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

	// 2. Error case (404)
	_, err = m.fetchNextActions(mockServer.URL, "invalid-token")
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

	// LocalAddrFn returns the mock server's host:port without http://
	m := Manager(cfg, printer.NewManager(), func() string {
		return mockLocal.Listener.Addr().String()
	})

	// 1. POST dispatch
	resPost := m.dispatchLocalAction("/test-post", "POST", map[string]string{"test_key": "test_val"})
	testutil.ExpectedEqual(t, receivedMethod, "POST")
	testutil.ExpectedEqual(t, receivedBody["test_key"], "test_val")
	resMap, ok := resPost.(map[string]interface{})
	testutil.ExpectedTrue(t, ok)
	testutil.ExpectedEqual(t, resMap["result"], "dispatched_ok")

	// 2. GET dispatch
	resGet := m.dispatchLocalAction("/test-get", "GET", nil)
	testutil.ExpectedEqual(t, receivedMethod, "GET")
	resMapGet, ok := resGet.(map[string]interface{})
	testutil.ExpectedTrue(t, ok)
	testutil.ExpectedEqual(t, resMapGet["result"], "dispatched_ok")
}

func TestObox_CallOdooOboxConnect(t *testing.T) {
	connectReceived := make(chan bool, 1)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/obox/connect" {
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
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"code": 404, "message": "not found"},
		})
	}))
	defer mockServer.Close()

	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)

	m := Manager(cfg, printer.NewManager(), func() string { return "127.0.0.1:4545" })
	defer m.Stop()

	m.callOdooOboxConnect(mockServer.URL, "conn-tok", "conn-uuid")

	select {
	case <-connectReceived:
		testutil.ExpectedTrue(t, m.IsConnected())
		dbURL, tok := m.GetCredentials()
		testutil.ExpectedEqual(t, dbURL, mockServer.URL)
		testutil.ExpectedEqual(t, tok, "conn-tok")
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
