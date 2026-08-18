package server

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"epos-proxy/internal/config"
	"epos-proxy/internal/obox"
	"epos-proxy/internal/printer"
	"epos-proxy/internal/testutil"
)

func createTestOboxServer(port int) *Server {
	cfg := &config.Manager{Data: config.AppConfig{Port: port}}
	s, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return s
}

func createTestOboxServerWithConfig(t *testing.T, port int) (*Server, *config.Manager) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)
	cfg.Data.Port = port
	s, err := New(cfg)
	testutil.ExpectedNoError(t, err)
	return s, cfg
}

func TestOboxDiscovery_Endpoint(t *testing.T) {
	s := createTestOboxServer(4601)
	defer s.Stop()

	// GET /odoo/ LAN discovery check
	req := httptest.NewRequest("GET", "/odoo/", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	var result struct {
		Status string                 `json:"status"`
		Data   map[string]interface{} `json:"data"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, result.Status, "not_configured")
}

func TestOboxConnectDB_Endpoint(t *testing.T) {
	s := createTestOboxServer(4602)
	defer s.Stop()

	// GET /odoo/connect
	req := httptest.NewRequest("GET", "/odoo/connect?db_url=http://127.0.0.1:8069&token=test-token-123&db_uuid=test-uuid", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	// Verify /odoo/ now returns the configured serial and db_url
	reqStatus := httptest.NewRequest("GET", "/odoo/", nil)
	respStatus, err := s.App().Test(reqStatus)
	testutil.ExpectedNoError(t, err)
	defer respStatus.Body.Close()

	var result struct {
		Status string            `json:"status"`
		Data   map[string]string `json:"data"`
	}
	err = json.NewDecoder(respStatus.Body).Decode(&result)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedNotNil(t, result.Data)
	testutil.ExpectedEqual(t, result.Data["db_url"], "http://127.0.0.1:8069")
}

func TestOboxOfflineConnect_Endpoint(t *testing.T) {
	s := createTestOboxServer(4603)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/connect?db_url=http://127.0.0.1:8069&db_uuid=test-uuid&token=test-tok", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
}

func TestOboxLANStatus_Endpoint(t *testing.T) {
	s := createTestOboxServer(4604)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
}

func TestOboxHealth_Endpoint(t *testing.T) {
	s := createTestOboxServer(4605)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/health", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	var res map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&res)
	testutil.ExpectedEqual(t, res["status"], "ok")
}

func TestOboxRestart_Endpoint(t *testing.T) {
	s := createTestOboxServer(4606)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/restart", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	var res map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&res)
	testutil.ExpectedEqual(t, res["status"], "restarted")
}

func TestOboxDisconnect_Endpoint(t *testing.T) {
	s := createTestOboxServer(4607)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)
}

func TestOboxDiscoverDevices_Endpoint(t *testing.T) {
	s := createTestOboxServer(4608)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/discover_devices", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	var devices []obox.DeviceEntry
	err = json.NewDecoder(resp.Body).Decode(&devices)
	testutil.ExpectedNoError(t, err)
	for _, d := range devices {
		testutil.ExpectedEqual(t, d.Type, "printer")
	}
}

func TestOboxRemoteDebug_Endpoints(t *testing.T) {
	s := createTestOboxServer(4609)
	defer s.Stop()

	// Enable
	req := httptest.NewRequest("GET", "/sos/v1/enable?token=tskey-auth-12345", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusBadRequest)

	// Disable
	req = httptest.NewRequest("GET", "/sos/v1/disable", nil)
	resp, err = s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusBadRequest)
}

func TestOboxPrinter_Endpoints(t *testing.T) {
	// Start mock TCP listener on port 9100 for LAN printer
	_, _, err := testutil.StartMockTCPServer(t, func(conn net.Conn) {
		defer conn.Close()
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
	})
	testutil.ExpectedNoError(t, err)

	s := createTestOboxServer(4612)
	defer s.Stop()

	printerID := printer.EncodeLANPrinterID("127.0.0.1")

	// 1. Generic print POST /usb/v1/printer/print
	printPayload := map[string]interface{}{
		"identifier": printerID,
		"document":   "bW9jay1kb2N1bWVudA==",
	}
	pBytes, _ := json.Marshal(printPayload)
	req := httptest.NewRequest("POST", "/usb/v1/printer/print", bytes.NewReader(pBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	// 2. ePOS print for LAN printer
	xmlPayload := `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><epos-print xmlns="http://www.epson-pos.com/schemas/2011/03/epos-print"><text>Hello</text></epos-print></s:Body></s:Envelope>`
	req = httptest.NewRequest("POST", "/usb/v1/printer/"+printerID+"/cgi-bin/epos/service.cgi", bytes.NewBufferString(xmlPayload))
	req.Header.Set("Content-Type", "application/xml")
	resp, err = s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	var eposResp EPOSResponse
	bodyBytes, _ := io.ReadAll(resp.Body)
	_ = xml.Unmarshal(bodyBytes, &eposResp)
	testutil.ExpectedTrue(t, eposResp.Success)

	// 3. Open cashbox
	req = httptest.NewRequest("GET", "/usb/v1/printer/open-cashbox?identifier="+printerID, nil)
	resp, err = s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	// 4. List printers
	req = httptest.NewRequest("GET", "/usb/v1/printer/list", nil)
	resp, err = s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()
	var pList [][]string
	err = json.NewDecoder(resp.Body).Decode(&pList)
	testutil.ExpectedNoError(t, err)
}

func TestOboxDisplay_Endpoint(t *testing.T) {
	s := createTestOboxServer(4613)
	defer s.Stop()

	body := map[string]string{"url": "https://odoo.com"}
	bBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/display/v1/update-url", bytes.NewReader(bBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusBadRequest)
}

func TestOboxWiFi_And_LED_Endpoints(t *testing.T) {
	s := createTestOboxServer(4614)
	defer s.Stop()

	// WiFi status
	req := httptest.NewRequest("GET", "/wifi/status", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusBadRequest)

	// WiFi networks
	req = httptest.NewRequest("GET", "/wifi/networks", nil)
	resp, err = s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusBadRequest)

	// LEDs set
	ledBody := []map[string]string{{"led": "led1", "color": "green"}}
	ledBytes, _ := json.Marshal(ledBody)
	req = httptest.NewRequest("POST", "/leds/set", bytes.NewReader(ledBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err = s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusBadRequest)
}

func TestExecuteAction_AllCases(t *testing.T) {
	// Start a mock Odoo server to receive /obox/action_result and /obox/ping
	actionReported := make(chan string, 15)
	mockOdoo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
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

	s := createTestOboxServer(4615)
	defer s.Stop()

	// Find the registered obox module
	m := s.Obox()

	m.SetCredentials(mockOdoo.URL, "test-token", "mock-uuid")

	testActions := []struct {
		name       string
		url        string
		method     string
		payload    interface{}
		actionUUID string
	}{
		{name: "Health", url: "/odoo/health", method: "GET", actionUUID: "uuid-1"},
		{name: "Restart", url: "/odoo/restart", method: "GET", actionUUID: "uuid-2"},
		{name: "DiscoverDevices", url: "/odoo/discover_devices", method: "GET", actionUUID: "uuid-3"},
		{name: "RemoteDebugEnable", url: "/sos/v1/enable?token=tskey-123", method: "GET", actionUUID: "uuid-4"},
		{name: "RemoteDebugDisable", url: "/sos/v1/disable", method: "GET", actionUUID: "uuid-5"},
		{name: "PrinterPrint", url: "/usb/v1/printer/print", method: "POST", payload: map[string]string{"identifier": "mock_device"}, actionUUID: "uuid-6"},
		{name: "ScaleWeight", url: "/usb/v1/scale/read_scale_weight", method: "POST", actionUUID: "uuid-7"},
		{name: "CameraPicture", url: "/usb/v1/camera/take-picture?identifier=mock", method: "GET", actionUUID: "uuid-8"},
		{name: "DisplayUpdate", url: "/display/v1/update-url", method: "POST", payload: map[string]string{"url": "https://odoo.com"}, actionUUID: "uuid-9"},
		{name: "LEDsSet", url: "/leds/set", method: "POST", actionUUID: "uuid-10"},
	}

	for _, tc := range testActions {
		t.Run(tc.name, func(t *testing.T) {
			action := obox.QueueAction{
				UUID: tc.actionUUID,
				Payload: map[string]interface{}{
					"url":     tc.url,
					"method":  tc.method,
					"payload": tc.payload,
				},
			}
			m.ExecuteAction(action)

			select {
			case uuid := <-actionReported:
				testutil.ExpectedEqual(t, uuid, tc.actionUUID)
			case <-time.After(2 * time.Second):
				t.Fatalf("Timeout waiting for action result reporting for %s", tc.name)
			}
		})
	}
}

func TestOboxStoragePersistence(t *testing.T) {
	s, cfg := createTestOboxServerWithConfig(t, 4616)
	defer s.Stop()

	testAppID := s.AppID()

	// 1. Initially no credentials in storage
	testutil.ExpectedFalse(t, cfg.HasOdooCredentials())

	// 2. Offline connect endpoint persists credentials to storage
	req := httptest.NewRequest("GET", "/odoo/connect?db_url=http://127.0.0.1:8069&db_uuid=test-uuid-456&token=test-tok-789", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	testutil.ExpectedTrue(t, cfg.HasOdooCredentials())
	testutil.ExpectedEqual(t, cfg.GetOdooDbURL(), "http://127.0.0.1:8069")
	testutil.ExpectedEqual(t, cfg.GetOdooToken(), "test-tok-789")
	testutil.ExpectedEqual(t, cfg.GetAppID(), testAppID)
	testutil.ExpectedEqual(t, cfg.GetOdooDbUUID(), "test-uuid-456")

	// 3. Disconnect clears credentials from storage
	reqDiscReq := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	respDiscReq, err := s.App().Test(reqDiscReq)
	testutil.ExpectedNoError(t, err)
	respDiscReq.Body.Close()

	testutil.ExpectedFalse(t, cfg.HasOdooCredentials())
}

func TestOboxRestoreCredentialsFromConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)
	testAppID := cfg.GetAppID()
	_ = cfg.SetOdooCredentials("http://192.168.1.100:8069", "restored-token", "uuid-1")

	cfg.Data.Port = 4617
	s, err := New(cfg)
	testutil.ExpectedNoError(t, err)
	defer s.Stop()

	// Hit /odoo/ to verify credentials were automatically restored into module
	req := httptest.NewRequest("GET", "/odoo/", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Status string            `json:"status"`
		Data   map[string]string `json:"data"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	testutil.ExpectedNoError(t, err)

	testutil.ExpectedNotNil(t, result.Data)
	testutil.ExpectedEqual(t, result.Data["serial"], testAppID)
	testutil.ExpectedEqual(t, result.Data["db_url"], "http://192.168.1.100:8069")
}

func TestGetOdooStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)

	cfg.Data.Port = 4618
	s, err := New(cfg)
	testutil.ExpectedNoError(t, err)
	defer s.Stop()

	// 1. Initial disconnected status
	testutil.ExpectedEqual(t, s.GetWebsocketStatus(), "disconnected")

	// 2. Connect via /odoo/connect
	reqConnect := httptest.NewRequest("GET", "/odoo/connect?db_url=http://127.0.0.1:8069&token=status-token-123&db_uuid=test-uuid", nil)
	respConnect, err := s.App().Test(reqConnect)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, respConnect.StatusCode, http.StatusOK)
	respConnect.Body.Close()

	// 3. Status should now be connected
	testutil.ExpectedEqual(t, s.GetOdooDbURL(), "http://127.0.0.1:8069")

	// 4. Disconnect via /odoo/disconnect
	reqDisc := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	respDisc, err := s.App().Test(reqDisc)
	testutil.ExpectedNoError(t, err)
	respDisc.Body.Close()

	// 5. Status should be disconnected again
	testutil.ExpectedEqual(t, s.GetWebsocketStatus(), "disconnected")
}
