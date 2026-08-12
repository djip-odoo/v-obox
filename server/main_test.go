package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"epos-proxy/printer"
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
