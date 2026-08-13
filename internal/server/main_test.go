package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"epos-proxy/internal/printer"
	"epos-proxy/internal/util"

	"github.com/gofiber/fiber/v3"
)

func TestAutoBindingAndRoutes(t *testing.T) {
	// Register a dynamic test module to prove auto-binding works for new modules
	customRouteHit := false
	Register(func(s *Server) {
		s.app.Get("/test-auto-bind", func(ctx fiber.Ctx) error {
			customRouteHit = true
			return ctx.SendString("auto-bind-success")
		})
	})

	mgr := printer.NewManager()
	s := New(4545, mgr)
	defer s.Stop()

	if !s.Running() {
		t.Errorf("Expected server to be running")
	}

	// Test Obox route registered via obox.go
	req := httptest.NewRequest("GET", "/odoo/health", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("Failed to test /odoo/health: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for /odoo/health, got %d", resp.StatusCode)
	}

	// Test custom auto-bound route
	reqCustom := httptest.NewRequest("GET", "/test-auto-bind", nil)
	respCustom, err := s.App().Test(reqCustom)
	if err != nil {
		t.Fatalf("Failed to test /test-auto-bind: %v", err)
	}
	if respCustom.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for /test-auto-bind, got %d", respCustom.StatusCode)
	}
	if !customRouteHit {
		t.Errorf("Expected customRouteHit to be true")
	}
}

func TestCustomSerialNumberSupport(t *testing.T) {
	mgr := printer.NewManager()
	s := New(4546, mgr)
	defer s.Stop()

	customSerial := "MY-CUSTOM-SERIAL-888"
	_ = s.Cfg().SetAppID(customSerial)

	// 1. Test /mock/connect with matching custom serial
	bodyPayload, _ := json.Marshal(map[string]string{
		"db_url": "http://127.0.0.1:8069",
		"token":  "test-token",
		"serial": customSerial,
	})
	reqConnect := httptest.NewRequest("POST", "/mock/connect", bytes.NewReader(bodyPayload))
	reqConnect.Header.Set("Content-Type", "application/json")
	respConnect, err := s.App().Test(reqConnect)
	if err != nil {
		t.Fatalf("POST /mock/connect failed: %v", err)
	}
	if respConnect.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 from /mock/connect, got %d", respConnect.StatusCode)
	}

	// 2. Test /mock/connect with mismatched serial returns 400 Bad Request
	mismatchPayload, _ := json.Marshal(map[string]string{
		"db_url": "http://127.0.0.1:8069",
		"token":  "test-token",
		"serial": "MISMATCHED-SERIAL-999",
	})
	reqMismatch := httptest.NewRequest("POST", "/mock/connect", bytes.NewReader(mismatchPayload))
	reqMismatch.Header.Set("Content-Type", "application/json")
	respMismatch, err := s.App().Test(reqMismatch)
	if err != nil {
		t.Fatalf("POST /mock/connect mismatch test failed: %v", err)
	}
	if respMismatch.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected 400 from /mock/connect with mismatched serial, got %d", respMismatch.StatusCode)
	}

	// 3. Test /mock/status returns the custom serial
	reqStatus := httptest.NewRequest("GET", "/mock/status", nil)
	respStatus, err := s.App().Test(reqStatus)
	if err != nil {
		t.Fatalf("GET /mock/status failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(respStatus.Body)
	if !strings.Contains(string(bodyBytes), customSerial) {
		t.Errorf("Expected status to contain %q, got: %s", customSerial, string(bodyBytes))
	}

	// 4. Test /odoo-enterprise/iot/discover-boxes with matching serial
	reqDiscover := httptest.NewRequest("GET", "/odoo-enterprise/iot/discover-boxes?serial="+customSerial, nil)
	respDiscover, err := s.App().Test(reqDiscover)
	if err != nil {
		t.Fatalf("GET /odoo-enterprise/iot/discover-boxes failed: %v", err)
	}
	discoverBytes, _ := io.ReadAll(respDiscover.Body)
	if !strings.Contains(string(discoverBytes), customSerial) {
		t.Errorf("Expected discover-boxes to contain %s, got: %s", customSerial, string(discoverBytes))
	}

	// 5. Test /odoo-enterprise/iot/discover-boxes with mismatched serial returns 400
	reqDiscoverBad := httptest.NewRequest("GET", "/odoo-enterprise/iot/discover-boxes?serial=BAD-SERIAL", nil)
	respDiscoverBad, err := s.App().Test(reqDiscoverBad)
	if err != nil {
		t.Fatalf("GET /odoo-enterprise/iot/discover-boxes bad test failed: %v", err)
	}
	if respDiscoverBad.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected 400 from discover-boxes with mismatched serial, got %d", respDiscoverBad.StatusCode)
	}
}

func TestServerLocalIPAndAddr(t *testing.T) {
	ip := util.GetLocalIP(false)
	if ip == "" {
		t.Errorf("Expected non-empty IP from GetLocalIP")
	}

	mgr := printer.NewManager()
	port := 4547
	s := New(port, mgr)
	defer s.Stop()

	if util.GetLocalIP(false) != ip {
		t.Errorf("Expected s.LocalIP() == %s, got %s", ip, util.GetLocalIP(false))
	}

	expectedAddr := fmt.Sprintf("%s:%d", ip, port)
	if s.LocalAddr() != expectedAddr {
		t.Errorf("Expected s.LocalAddr() == %s, got %s", expectedAddr, s.LocalAddr())
	}
}
