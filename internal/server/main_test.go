package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"epos-proxy/internal/printer"

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
	s := New(4545, mgr, nil)
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
	s := New(4546, mgr, nil)
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

	// 4. Test /odoo/discover_devices endpoint
	reqDiscover := httptest.NewRequest("GET", "/odoo/discover_devices", nil)
	respDiscover, err := s.App().Test(reqDiscover)
	if err != nil {
		t.Fatalf("GET /odoo/discover_devices failed: %v", err)
	}
	if respDiscover.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 from /odoo/discover_devices, got %d", respDiscover.StatusCode)
	}
	respDiscover.Body.Close()
}
