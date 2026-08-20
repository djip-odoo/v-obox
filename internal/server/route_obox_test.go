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

	"epos-proxy/internal/config"
	"epos-proxy/internal/printer"
	"epos-proxy/internal/testutil"
)

func createTestOboxServer(t testing.TB) *Server {
	t.Helper()
	port := testutil.GetFreePort(t)
	cfg := &config.Manager{Data: config.AppConfig{Port: port}}
	s, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return s
}

func createTestOboxServerWithConfig(t *testing.T) (*Server, *config.Manager) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)
	cfg.Data.Port = testutil.GetFreePort(t)
	s, err := New(cfg)
	testutil.ExpectedNoError(t, err)
	return s, cfg
}

func TestOboxDiscovery_Endpoint(t *testing.T) {
	s := createTestOboxServer(t)
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
	s := createTestOboxServer(t)
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

func TestOboxPrinter_Endpoints(t *testing.T) {
	// Start mock TCP listener on port 9100 for LAN printer
	_, _, err := testutil.StartMockTCPServer(t, func(conn net.Conn) {
		defer conn.Close()
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
	})
	testutil.ExpectedNoError(t, err)

	s := createTestOboxServer(t)
	defer s.Stop()

	printerID := printer.EncodeLANPrinterID("127.0.0.1")

	// 1. ePOS print for LAN printer
	xmlPayload := `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><epos-print xmlns="http://www.epson-pos.com/schemas/2011/03/epos-print"><text>Hello</text></epos-print></s:Body></s:Envelope>`
	req := httptest.NewRequest("POST", "/usb/v1/printer/"+printerID+"/cgi-bin/epos/service.cgi", bytes.NewBufferString(xmlPayload))
	req.Header.Set("Content-Type", "application/xml")
	resp, err := s.App().Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	var eposResp EPOSResponse
	_ = xml.Unmarshal(bodyBytes, &eposResp)
	testutil.ExpectedTrue(t, eposResp.Success)
}

func TestOboxUnsupportedEndpoints(t *testing.T) {
	s := createTestOboxServer(t)
	defer s.Stop()

	unsupportedRoutes := []struct {
		method  string
		path    string
		payload string
	}{
		{method: "GET", path: "/sos/v1/enable?token=tskey-auth-12345"},
		{method: "GET", path: "/sos/v1/disable"},
		{method: "POST", path: "/display/v1/update-url", payload: `{"url":"https://odoo.com"}`},
		{method: "GET", path: "/wifi/status"},
		{method: "GET", path: "/wifi/networks"},
		{method: "POST", path: "/leds/set", payload: `[{"led":"led1","color":"green"}]`},
		{method: "POST", path: "/usb/v1/printer/print", payload: `{"identifier":"mock_device","document":"bW9jay1kb2N1bWVudA=="}`},
	}

	for _, tc := range unsupportedRoutes {
		var req *http.Request
		if tc.payload != "" {
			req = httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.payload))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(tc.method, tc.path, nil)
		}
		resp, err := s.App().Test(req)
		testutil.ExpectedNoError(t, err)
		testutil.ExpectedEqual(t, resp.StatusCode, http.StatusBadRequest)
		resp.Body.Close()
	}
}

func TestOboxStoragePersistence(t *testing.T) {
	s, cfg := createTestOboxServerWithConfig(t)
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

	cfg.Data.Port = testutil.GetFreePort(t)
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

	cfg.Data.Port = testutil.GetFreePort(t)
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
