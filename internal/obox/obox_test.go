package obox

import (
	"encoding/json"
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

	m := New(cfg, func() string { return "127.0.0.1:4545" })
	m.RegisterRoutes(app)
	return m, app
}

func TestObox_CredentialsAndConnection(t *testing.T) {
	m, _ := createTestModule(t)

	testutil.ExpectedFalse(t, m.IsConnected())
	testutil.ExpectedEqual(t, m.GetWebsocketStatus(), "disconnected")

	m.SetCredentials("http://127.0.0.1:8069", "token-xyz", "db-uuid-1")
	testutil.ExpectedTrue(t, m.IsConnected())

	dbURL, tok, uuid := m.GetCredentials()
	testutil.ExpectedEqual(t, dbURL, "http://127.0.0.1:8069")
	testutil.ExpectedEqual(t, tok, "token-xyz")
	testutil.ExpectedEqual(t, uuid, "db-uuid-1")
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

	// 1. Initial /odoo/
	req := httptest.NewRequest("GET", "/odoo/", nil)
	resp, err := app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	// 2. /odoo/health
	req = httptest.NewRequest("GET", "/odoo/health", nil)
	resp, err = app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	// 3. /odoo/restart
	req = httptest.NewRequest("GET", "/odoo/restart", nil)
	resp, err = app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	// 4. /odoo/connect
	req = httptest.NewRequest("GET", "/odoo/connect?db_url=http://127.0.0.1:8069&token=test-tok&db_uuid=test-uuid", nil)
	resp, err = app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	testutil.ExpectedTrue(t, m.IsConnected())

	// 5. /odoo/ configured discovery
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

	// 6. /odoo/disconnect
	req = httptest.NewRequest("GET", "/odoo/disconnect", nil)
	resp, err = app.Test(req)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
	resp.Body.Close()

	testutil.ExpectedFalse(t, m.IsConnected())
}

func TestObox_ExecuteAction(t *testing.T) {
	actionReported := make(chan string, 5)
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
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": "ok"})
	}))
	defer mockOdoo.Close()

	m, _ := createTestModule(t)
	m.SetCredentials(mockOdoo.URL, "tok", "uuid")

	action := QueueAction{
		UUID: "action-1",
		Payload: map[string]interface{}{
			"url":    "/odoo/health",
			"method": "GET",
		},
	}
	m.ExecuteAction(action)

	select {
	case uuid := <-actionReported:
		testutil.ExpectedEqual(t, uuid, "action-1")
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for action report")
	}
}
