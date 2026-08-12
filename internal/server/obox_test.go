package server

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"epos-proxy/internal/printer"
)

func createTestOboxServer(port int) *Server {
	mgr := printer.NewManager()
	return New(port, mgr)
}

func TestOboxDiscovery_Endpoint(t *testing.T) {
	s := createTestOboxServer(4601)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo-enterprise/iot/discover-boxes", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var boxes []map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&boxes); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	if len(boxes) == 0 {
		t.Fatal("Expected at least one discoverable box")
	}

	hasODO := false
	for _, b := range boxes {
		if sn, ok := b["serial_number"]; ok && strings.HasPrefix(sn, "ODO-") {
			hasODO = true
			break
		}
	}
	if !hasODO {
		t.Fatal("Expected at least one box with serial number starting with ODO-")
	}
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
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var res struct {
		Result []map[string]string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(res.Result) == 0 || res.Result[0]["serial_number"] == "" {
		t.Fatal("Expected serial_number in result")
	}
}

func TestOboxOfflineConnect_Endpoint(t *testing.T) {
	s := createTestOboxServer(4603)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/connect?db_url=http://127.0.0.1:8069&db_uuid=test-uuid&token=test-tok", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOboxLANStatus_Endpoint(t *testing.T) {
	s := createTestOboxServer(4604)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOboxHealth_Endpoint(t *testing.T) {
	s := createTestOboxServer(4605)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/health", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if res["status"] != "ok" {
		t.Fatalf("Expected status 'ok', got '%s'", res["status"])
	}
}

func TestOboxRestart_Endpoint(t *testing.T) {
	s := createTestOboxServer(4606)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/restart", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var res map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if res["status"] != "restarted" {
		t.Fatalf("Expected status 'restarted', got '%s'", res["status"])
	}
}

func TestOboxDisconnect_Endpoint(t *testing.T) {
	s := createTestOboxServer(4607)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/disconnect", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOboxDiscoverDevices_Endpoint(t *testing.T) {
	s := createTestOboxServer(4608)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/odoo/discover_devices", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var devices []oboxDeviceEntry
	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		t.Fatalf("Failed to decode devices list: %v", err)
	}

	foundVirtualPrinter := false
	for _, d := range devices {
		if d.Identifier == virtualPrinterID && d.Type == "printer" {
			foundVirtualPrinter = true
		}
		if d.Type != "printer" {
			t.Fatalf("Unexpected device type '%s', only printers should be added", d.Type)
		}
	}

	if !foundVirtualPrinter {
		t.Fatal("Expected Virtual POS Receipt Printer in discovered devices")
	}
}

func TestOboxRemoteDebug_Endpoints(t *testing.T) {
	s := createTestOboxServer(4609)
	defer s.Stop()

	// Enable
	req := httptest.NewRequest("GET", "/sos/v1/enable?token=tskey-auth-12345", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	var res map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&res)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || res["status"] == "" {
		t.Fatalf("Expected enable status ok, got %v", res)
	}

	// Disable
	req = httptest.NewRequest("GET", "/sos/v1/disable", nil)
	resp, err = s.App().Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || res["status"] == "" {
		t.Fatalf("Expected disable status ok, got %v", res)
	}
}

func TestOboxScale_Endpoints(t *testing.T) {
	s := createTestOboxServer(4610)
	defer s.Stop()

	// Set custom weight via mock API
	scaleReqBody := map[string]float64{"weight": 3.750}
	scaleBytes, _ := json.Marshal(scaleReqBody)
	req := httptest.NewRequest("POST", "/mock/scale", bytes.NewReader(scaleBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("POST /mock/scale failed: %v", err)
	}
	resp.Body.Close()

	// Read scale weight
	readBody := map[string]interface{}{"identifier": "usb_scale_mock", "unit_price": 4.5}
	readBytes, _ := json.Marshal(readBody)
	req = httptest.NewRequest("POST", "/usb/v1/scale/read_scale_weight", bytes.NewReader(readBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err = s.App().Test(req)
	if err != nil {
		t.Fatalf("POST /usb/v1/scale/read_scale_weight failed: %v", err)
	}
	defer resp.Body.Close()

	var readRes struct {
		Weight float64 `json:"weight"`
		Unit   string  `json:"unit"`
		Status string  `json:"status"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&readRes)
	if readRes.Weight != 3.750 || readRes.Unit != "kg" || readRes.Status != "ok" {
		t.Fatalf("Unexpected scale read response: %+v", readRes)
	}
}

func TestOboxCamera_Endpoints(t *testing.T) {
	s := createTestOboxServer(4611)
	defer s.Stop()

	req := httptest.NewRequest("GET", "/usb/v1/camera/take-picture?identifier=mock_cam", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("GET /usb/v1/camera/take-picture failed: %v", err)
	}
	defer resp.Body.Close()

	var res struct {
		Image string `json:"image"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)
	if res.Image == "" {
		t.Fatal("Expected non-empty base64 image data")
	}
}

func TestOboxPrinter_Endpoints(t *testing.T) {
	s := createTestOboxServer(4612)
	defer s.Stop()

	// 1. Generic print POST /usb/v1/printer/print
	printPayload := map[string]interface{}{
		"identifier": virtualPrinterID,
		"document":   "bW9jay1kb2N1bWVudA==",
	}
	pBytes, _ := json.Marshal(printPayload)
	req := httptest.NewRequest("POST", "/usb/v1/printer/print", bytes.NewReader(pBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("POST /usb/v1/printer/print failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	// 2. ePOS print for virtual printer
	xmlPayload := `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><epos-print xmlns="http://www.epson-pos.com/schemas/2011/03/epos-print"><text>Hello</text></epos-print></s:Body></s:Envelope>`
	req = httptest.NewRequest("POST", fmt.Sprintf("/usb/v1/printer/%s/cgi-bin/epos/service.cgi", virtualPrinterID), bytes.NewBufferString(xmlPayload))
	req.Header.Set("Content-Type", "application/xml")
	resp, err = s.App().Test(req)
	if err != nil {
		t.Fatalf("ePOS print failed: %v", err)
	}
	defer resp.Body.Close()

	var eposResp EPOSResponse
	bodyBytes, _ := io.ReadAll(resp.Body)
	_ = xml.Unmarshal(bodyBytes, &eposResp)
	if !eposResp.Success {
		t.Fatalf("Expected ePOS success, got %+v", eposResp)
	}

	// 3. Open cashbox
	req = httptest.NewRequest("GET", "/usb/v1/printer/open-cashbox?identifier="+virtualPrinterID, nil)
	resp, err = s.App().Test(req)
	if err != nil {
		t.Fatalf("open cashbox failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	// 4. List printers
	req = httptest.NewRequest("GET", "/usb/v1/printer/list", nil)
	resp, err = s.App().Test(req)
	if err != nil {
		t.Fatalf("list printers failed: %v", err)
	}
	defer resp.Body.Close()
	var pList [][]string
	_ = json.NewDecoder(resp.Body).Decode(&pList)
	if len(pList) == 0 {
		t.Fatal("Expected at least one printer in list")
	}
}

func TestOboxDisplay_Endpoint(t *testing.T) {
	s := createTestOboxServer(4613)
	defer s.Stop()

	body := map[string]string{"url": "https://odoo.com"}
	bBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/display/v1/update-url", bytes.NewReader(bBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("POST /display/v1/update-url failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestOboxWiFi_And_LED_Endpoints(t *testing.T) {
	s := createTestOboxServer(4614)
	defer s.Stop()

	// WiFi status
	req := httptest.NewRequest("GET", "/wifi/status", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("GET /wifi/status failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	// WiFi networks
	req = httptest.NewRequest("GET", "/wifi/networks", nil)
	resp, err = s.App().Test(req)
	if err != nil {
		t.Fatalf("GET /wifi/networks failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}

	// LEDs set
	ledBody := []map[string]string{{"led": "led1", "color": "green"}}
	ledBytes, _ := json.Marshal(ledBody)
	req = httptest.NewRequest("POST", "/leds/set", bytes.NewReader(ledBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err = s.App().Test(req)
	if err != nil {
		t.Fatalf("POST /leds/set failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200, got %d", resp.StatusCode)
	}
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
		serial: "12345",
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
		{name: "PrinterPrint", url: "/usb/v1/printer/print", method: "POST", payload: map[string]string{"identifier": virtualPrinterID}, actionUUID: "uuid-6"},
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
				if uuid != tc.actionUUID {
					t.Fatalf("Expected reported uuid %s, got %s", tc.actionUUID, uuid)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("Timeout waiting for action result reporting for %s", tc.name)
			}
		})
	}
}
