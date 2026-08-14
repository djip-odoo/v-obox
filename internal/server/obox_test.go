package server

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"epos-proxy/internal/config"
	"epos-proxy/internal/printer"
	"epos-proxy/testutil"
)

func createTestOboxServer(port int) *Server {
	mgr := printer.NewManager()
	return New(port, mgr)
}

func createTestOboxServerWithConfig(t *testing.T, port int) (*Server, *config.Manager) {
	tempDir := t.TempDir()
	cfg := config.NewManagerWithPath(filepath.Join(tempDir, "config.json"))
	mgr := printer.NewManager()
	return New(port, mgr, cfg), cfg
}

func TestOboxDiscovery_Endpoint(t *testing.T) {
	s := createTestOboxServer(4601)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo-enterprise/iot/discover-boxes", nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	var boxes []map[string]string
	err = json.NewDecoder(resp.Body).Decode(&boxes)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedTrue(t, len(boxes) > 0)

	foundAppID := false
	for _, b := range boxes {
		if sn, ok := b["serial_number"]; ok && sn == s.AppID() {
			foundAppID = true
			break
		}
	}
	testutil.ExpectedTrue(t, foundAppID, "Expected discoverable box with serial %s, got: %+v", s.AppID(), boxes)
}

func TestOboxConnectDB_Endpoint(t *testing.T) {
	s := createTestOboxServer(4602)
	defer s.Stop()

	body := map[string]interface{}{
		"params": map[string]string{
			"pairing_code": "MOCKPAIR01",
			"database_url": "http://127.0.0.1:8069",
			"token":        "test-token-123",
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/odoo-enterprise/iot/connect-db", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	var res struct {
		Result []map[string]string `json:"result"`
	}
	err = json.NewDecoder(resp.Body).Decode(&res)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedTrue(t, len(res.Result) > 0 && res.Result[0]["serial_number"] != "")
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

	var devices []oboxDeviceEntry
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
	s := createTestOboxServer(4612)
	defer s.Stop()

	// 1. Generic print POST /usb/v1/printer/print
	printPayload := map[string]interface{}{
		"identifier": "mock_device",
		"document":   "bW9jay1kb2N1bWVudA==",
	}
	pBytes, _ := json.Marshal(printPayload)
	req := httptest.NewRequest("POST", "/usb/v1/printer/print", bytes.NewReader(pBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	// 2. ePOS print for mock device
	xmlPayload := `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><epos-print xmlns="http://www.epson-pos.com/schemas/2011/03/epos-print"><text>Hello</text></epos-print></s:Body></s:Envelope>`
	req = httptest.NewRequest("POST", "/usb/v1/printer/mock_device/cgi-bin/epos/service.cgi", bytes.NewBufferString(xmlPayload))
	req.Header.Set("Content-Type", "application/xml")
	resp, err = s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	var eposResp EPOSResponse
	bodyBytes, _ := io.ReadAll(resp.Body)
	_ = xml.Unmarshal(bodyBytes, &eposResp)
	testutil.ExpectedTrue(t, eposResp.Success)

	// 3. Open cashbox
	req = httptest.NewRequest("GET", "/usb/v1/printer/open-cashbox?identifier=mock_device", nil)
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
	m := &oboxModule{server: s}
	m.setMockWeight(1.250)

	dev := &oboxDevice{
		dbURL:  mockOdoo.URL,
		token:  "test-token",
		serial: s.AppID(),
	}
	m.device.Store(dev)

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
			action := queueAction{
				UUID: tc.actionUUID,
				Payload: map[string]interface{}{
					"url":     tc.url,
					"method":  tc.method,
					"payload": tc.payload,
				},
			}
			m.executeAction(dev, action)

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

	// 2. Test mismatched serial returns error 400
	reqBad := httptest.NewRequest("GET", "/odoo/connect?db_url=http://127.0.0.1:8069&db_uuid=test-uuid-456&token=test-tok-789&serial_number=WRONG-SERIAL", nil)
	respBad, err := s.App().Test(reqBad)
	testutil.ExpectedNoError(t, err)
	respBad.Body.Close()
	testutil.ExpectedEqual(t, respBad.StatusCode, http.StatusBadRequest)

	// 3. Offline connect endpoint persists credentials to storage when serial matches AppID
	req := httptest.NewRequest("GET", "/odoo/connect?db_url=http://127.0.0.1:8069&db_uuid=test-uuid-456&token=test-tok-789&serial_number="+testAppID, nil)
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	testutil.ExpectedTrue(t, cfg.HasOdooCredentials())
	testutil.ExpectedEqual(t, cfg.GetOdooDbURL(), "http://127.0.0.1:8069")
	testutil.ExpectedEqual(t, cfg.GetOdooToken(), "test-tok-789")
	testutil.ExpectedEqual(t, cfg.GetOdooSerial(), testAppID)
	testutil.ExpectedEqual(t, cfg.GetOdooDbUUID(), "test-uuid-456")

	// 4. Discover boxes returns matching serial
	reqDisc := httptest.NewRequest("GET", "/odoo-enterprise/iot/discover-boxes", nil)
	respDisc, err := s.App().Test(reqDisc)
	testutil.ExpectedNoError(t, err)
	defer respDisc.Body.Close()
	var boxes []map[string]string
	_ = json.NewDecoder(respDisc.Body).Decode(&boxes)
	foundStored := false
	for _, b := range boxes {
		if b["serial_number"] == testAppID {
			foundStored = true
			break
		}
	}
	testutil.ExpectedTrue(t, foundStored, "Expected stored serial in discover boxes result")

	// 5. Disconnect clears credentials from storage
	reqDiscReq := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	respDiscReq, err := s.App().Test(reqDiscReq)
	testutil.ExpectedNoError(t, err)
	respDiscReq.Body.Close()

	testutil.ExpectedFalse(t, cfg.HasOdooCredentials())
}

func TestOboxRestoreCredentialsFromConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	cfg := config.NewManagerWithPath(configPath)
	testAppID, _ := cfg.EnsureAppID()
	_ = cfg.SetOdooCredentials("http://192.168.1.100:8069", "restored-token", testAppID, "uuid-1")

	mgr := printer.NewManager()
	s := New(4617, mgr, cfg)
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
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	cfg := config.NewManagerWithPath(configPath)
	testAppID, _ := cfg.EnsureAppID()

	mgr := printer.NewManager()
	s := New(4618, mgr, cfg)
	defer s.Stop()

	// 1. Initial disconnected status
	status := s.GetOdooStatus()
	testutil.ExpectedFalse(t, status.Connected)
	testutil.ExpectedEqual(t, status.WebsocketStatus, "disconnected")
	testutil.ExpectedEqual(t, status.Serial, testAppID)

	// 2. Connect via /mock/connect
	bodyPayload, _ := json.Marshal(map[string]string{
		"db_url": "http://127.0.0.1:8069",
		"token":  "status-token-123",
		"serial": testAppID,
	})
	reqConnect := httptest.NewRequest("POST", "/mock/connect", bytes.NewReader(bodyPayload))
	reqConnect.Header.Set("Content-Type", "application/json")
	respConnect, err := s.App().Test(reqConnect)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, respConnect.StatusCode, http.StatusOK)
	respConnect.Body.Close()

	// 3. Status should now be connected
	statusConnected := s.GetOdooStatus()
	testutil.ExpectedTrue(t, statusConnected.Connected)
	testutil.ExpectedEqual(t, statusConnected.DbURL, "http://127.0.0.1:8069")
	testutil.ExpectedEqual(t, statusConnected.WebsocketStatus, "connected")
	testutil.ExpectedEqual(t, statusConnected.Serial, testAppID)

	// 4. Disconnect via /odoo/disconnect
	reqDisc := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	respDisc, err := s.App().Test(reqDisc)
	testutil.ExpectedNoError(t, err)
	respDisc.Body.Close()

	// 5. Status should be disconnected again
	statusDisconnected := s.GetOdooStatus()
	testutil.ExpectedFalse(t, statusDisconnected.Connected)
	testutil.ExpectedEqual(t, statusDisconnected.WebsocketStatus, "disconnected")
}
