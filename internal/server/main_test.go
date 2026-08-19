package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"epos-proxy/internal/config"

	"github.com/gofiber/fiber/v3"
)

func TestServerRoutes(t *testing.T) {
	customRouteHit := false
	RegisterRoute(func(s *Server, cfg *config.Manager) {
		s.app.Get("/test-custom-route", func(ctx fiber.Ctx) error {
			customRouteHit = true
			return ctx.SendString("ok")
		})
	})

	cfg := &config.Manager{Data: config.AppConfig{Port: 4545}}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer s.Stop()

	if !s.Running() {
		t.Errorf("Expected server to be running")
	}

	// Test custom route registered via RegisterRoute
	reqCustom := httptest.NewRequest("GET", "/test-custom-route", nil)
	respCustom, err := s.App().Test(reqCustom)
	if err != nil {
		t.Fatalf("Failed to test /test-custom-route: %v", err)
	}
	if respCustom.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for /test-custom-route, got %d", respCustom.StatusCode)
	}
	if !customRouteHit {
		t.Errorf("Expected customRouteHit to be true")
	}

	// Test Obox route registered via route_obox.go
	req := httptest.NewRequest("GET", "/odoo/", nil)
	resp, err := s.App().Test(req)
	if err != nil {
		t.Fatalf("Failed to test /odoo/: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for /odoo/, got %d", resp.StatusCode)
	}

	// Test ePOS route registered via route_epos.go
	reqEPOS := httptest.NewRequest("POST", "/cgi-bin/epos/service.cgi", strings.NewReader("<invalid></invalid>"))
	reqEPOS.Header.Set("Content-Type", "text/xml")
	respEPOS, err := s.App().Test(reqEPOS)
	if err != nil {
		t.Fatalf("Failed to test ePOS endpoint: %v", err)
	}
	if respEPOS.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for ePOS route, got %d", respEPOS.StatusCode)
	}
}

func TestCustomSerialNumberSupport(t *testing.T) {
	cfg := &config.Manager{Data: config.AppConfig{Port: 4546, AppID: "CUSTOM_SERIAL_123"}}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer s.Stop()

	serverSerial := s.AppID()
	if serverSerial == "" {
		t.Fatalf("Expected non-empty server serial AppID")
	}

	// 1. Initial /odoo/ check (unconfigured)
	reqInit := httptest.NewRequest("GET", "/odoo/", nil)
	respInit, err := s.App().Test(reqInit)
	if err != nil {
		t.Fatalf("GET /odoo/ failed: %v", err)
	}
	if respInit.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 from /odoo/, got %d", respInit.StatusCode)
	}

	// 2. Connect via /odoo/connect
	reqConnect := httptest.NewRequest("GET", "/odoo/connect?db_url=http://127.0.0.1:8069&token=test-token&db_uuid=test-uuid", nil)
	respConnect, err := s.App().Test(reqConnect)
	if err != nil {
		t.Fatalf("GET /odoo/connect failed: %v", err)
	}
	if respConnect.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 from /odoo/connect, got %d", respConnect.StatusCode)
	}

	// 3. Test /odoo/ returns the custom serial
	reqStatus := httptest.NewRequest("GET", "/odoo/", nil)
	respStatus, err := s.App().Test(reqStatus)
	if err != nil {
		t.Fatalf("GET /odoo/ failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(respStatus.Body)
	if !strings.Contains(string(bodyBytes), serverSerial) {
		t.Errorf("Expected /odoo/ response to contain %q, got: %s", serverSerial, string(bodyBytes))
	}

	// 4. Test /usb/v1/printer/list endpoint
	reqDiscover := httptest.NewRequest("GET", "/usb/v1/printer/list", nil)
	respDiscover, err := s.App().Test(reqDiscover)
	if err != nil {
		t.Fatalf("GET /usb/v1/printer/list failed: %v", err)
	}
	if respDiscover.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 from /usb/v1/printer/list, got %d", respDiscover.StatusCode)
	}
	respDiscover.Body.Close()
}
