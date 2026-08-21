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
	"epos-proxy/internal/printer"
	"epos-proxy/internal/testutil"
)

func TestOboxDiscovery_Endpoint(t *testing.T) {
	s, _ := newTestServer(t, nil)

	req := httptest.NewRequest("GET", "/odoo/", nil)
	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	var result struct {
		Status string `json:"status"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, result.Status, "not_configured")
}

func TestOboxConnectDB_Endpoint(t *testing.T) {
	mockOdoo := newMockOdoo(t)
	s, _ := newTestServer(t, nil)

	connectURL := "/odoo/connect?db_url=" + mockOdoo.URL + "&token=test-token-123&db_uuid=test-uuid"
	req := httptest.NewRequest("GET", connectURL, nil)
	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	reqStatus := httptest.NewRequest("GET", "/odoo/", nil)
	respStatus, err := s.app.Test(reqStatus)
	testutil.ExpectedNoError(t, err)
	defer respStatus.Body.Close()

	var result struct {
		Status string            `json:"status"`
		Data   map[string]string `json:"data"`
	}
	err = json.NewDecoder(respStatus.Body).Decode(&result)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedNotNil(t, result.Data)
	testutil.ExpectedEqual(t, result.Data["db_url"], mockOdoo.URL)

	reqDisc := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	respDisc, err := s.app.Test(reqDisc)
	testutil.ExpectedNoError(t, err)
	respDisc.Body.Close()
}

func TestOboxPrinter_Endpoints(t *testing.T) {
	_, _, err := testutil.StartMockTCPServer(t, func(conn net.Conn) {
		defer conn.Close()
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)
	})
	testutil.ExpectedNoError(t, err)

	s, _ := newTestServer(t, nil)
	printerID := printer.EncodeLANPrinterID("127.0.0.1")

	xmlPayload := `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><epos-print xmlns="http://www.epson-pos.com/schemas/2011/03/epos-print"><text>Hello</text></epos-print></s:Body></s:Envelope>`
	req := httptest.NewRequest("POST", "/usb/v1/printer/"+printerID+"/cgi-bin/epos/service.cgi", bytes.NewBufferString(xmlPayload))
	req.Header.Set("Content-Type", "application/xml")
	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	var eposResp EPOSResponse
	_ = xml.Unmarshal(bodyBytes, &eposResp)
	testutil.ExpectedTrue(t, eposResp.Success)
}

func TestOboxUnsupportedEndpoints(t *testing.T) {
	s, _ := newTestServer(t, nil)

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
		resp, err := s.app.Test(req)
		testutil.ExpectedNoError(t, err)
		testutil.ExpectedEqual(t, resp.StatusCode, http.StatusBadRequest)
		resp.Body.Close()
	}
}

func TestOboxStoragePersistence(t *testing.T) {
	mockOdoo := newMockOdoo(t)
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)

	s, _ := newTestServer(t, cfg)
	testAppID := s.AppID()

	// 1. Initially no credentials
	testutil.ExpectedFalse(t, cfg.HasOdooCredentials())

	// 2. Connect persists credentials
	connectURL := "/odoo/connect?db_url=" + mockOdoo.URL + "&db_uuid=test-uuid-456&token=test-tok-789"
	req := httptest.NewRequest("GET", connectURL, nil)
	resp, err := s.app.Test(req)
	testutil.ExpectedNoError(t, err)
	resp.Body.Close()
	testutil.ExpectedEqual(t, resp.StatusCode, http.StatusOK)

	testutil.ExpectedTrue(t, cfg.HasOdooCredentials())
	testutil.ExpectedEqual(t, cfg.GetOdooDbURL(), mockOdoo.URL)
	testutil.ExpectedEqual(t, cfg.GetOdooToken(), "test-tok-789")
	testutil.ExpectedEqual(t, cfg.GetAppID(), testAppID)
	testutil.ExpectedEqual(t, cfg.GetOdooDbUUID(), "test-uuid-456")

	// 3. Disconnect clears credentials
	reqDisc := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	respDisc, err := s.app.Test(reqDisc)
	testutil.ExpectedNoError(t, err)
	respDisc.Body.Close()

	testutil.ExpectedFalse(t, cfg.HasOdooCredentials())
}

func TestOboxRestoreCredentialsFromConfig(t *testing.T) {
	mockOdoo := newMockOdoo(t)
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)
	testAppID := cfg.GetAppID()
	_ = cfg.SetOdooCredentials(mockOdoo.URL, "restored-token", "uuid-1")

	s, _ := newTestServer(t, cfg)

	req := httptest.NewRequest("GET", "/odoo/", nil)
	resp, err := s.app.Test(req)
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
	testutil.ExpectedEqual(t, result.Data["db_url"], mockOdoo.URL)

	reqDisc := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	respDisc, err := s.app.Test(reqDisc)
	testutil.ExpectedNoError(t, err)
	respDisc.Body.Close()
}

func TestGetOdooStatus(t *testing.T) {
	mockOdoo := newMockOdoo(t)
	s, _ := newTestServer(t, nil)

	// 1. Disconnected
	testutil.ExpectedEqual(t, s.GetWebsocketStatus(), "disconnected")

	// 2. Connect
	connectURL := "/odoo/connect?db_url=" + mockOdoo.URL + "&token=status-token-123&db_uuid=test-uuid"
	reqConnect := httptest.NewRequest("GET", connectURL, nil)
	respConnect, err := s.app.Test(reqConnect)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, respConnect.StatusCode, http.StatusOK)
	respConnect.Body.Close()

	// 3. Verify db_url
	testutil.ExpectedEqual(t, s.GetOdooDbURL(), mockOdoo.URL)
	for range 50 {
		if s.GetWebsocketStatus() == "connected" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 4. Disconnect
	reqDisc := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	respDisc, err := s.app.Test(reqDisc)
	testutil.ExpectedNoError(t, err)
	respDisc.Body.Close()

	testutil.ExpectedEqual(t, s.GetWebsocketStatus(), "disconnected")
}
