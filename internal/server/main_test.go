package server

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"epos-proxy/internal/config"
	"epos-proxy/internal/testutil"
)

func TestNew_NilConfig(t *testing.T) {
	s, err := New(nil)
	testutil.ExpectedError(t, err)
	testutil.ExpectedContains(t, err.Error(), "config manager is required")
	testutil.ExpectedNil(t, s)
}

func TestNew_PortResolutionError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)

	var listeners []net.Listener
	for p := config.PortRangeStart; p <= config.PortRangeEnd; p++ {
		var ln net.Listener
		var err error
		for range 30 {
			ln, err = net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", p))
			if err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		testutil.ExpectedNoError(t, err)
		listeners = append(listeners, ln)
	}
	defer func() {
		for _, ln := range listeners {
			_ = ln.Close()
		}
	}()

	s, err := New(cfg)
	testutil.ExpectedError(t, err)
	testutil.ExpectedContains(t, err.Error(), "unable to start server")
	testutil.ExpectedNil(t, s)
}

func TestServer_Lifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.NewManager()
	testutil.ExpectedNoError(t, err)
	port := testutil.GetFreePort(t)
	cfg.Data.Port = port

	s, err := New(cfg)
	testutil.ExpectedNoError(t, err)
	defer s.Stop()

	testutil.ExpectedTrue(t, s.Running(), "Expected server to be running after New()")
	testutil.ExpectedEqual(t, s.Port, port)
}

func TestServerRoutes(t *testing.T) {
	port := testutil.GetFreePort(t)
	cfg := &config.Manager{Data: config.AppConfig{Port: port}}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	defer s.Stop()

	if !s.Running() {
		t.Errorf("Expected server to be running")
	}

	// Test Obox route registered via registerOboxRoutes
	req := httptest.NewRequest("GET", "/odoo/", nil)
	resp, err := s.app.Test(req)
	if err != nil {
		t.Fatalf("Failed to test /odoo/: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for /odoo/, got %d", resp.StatusCode)
	}

	// Test ePOS route registered via registerEPOSRoutes
	reqEPOS := httptest.NewRequest("POST", "/cgi-bin/epos/service.cgi", strings.NewReader("<invalid></invalid>"))
	reqEPOS.Header.Set("Content-Type", "text/xml")
	respEPOS, err := s.app.Test(reqEPOS)
	if err != nil {
		t.Fatalf("Failed to test ePOS endpoint: %v", err)
	}
	if respEPOS.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for ePOS route, got %d", respEPOS.StatusCode)
	}
}

func TestCustomSerialNumberSupport(t *testing.T) {
	port := testutil.GetFreePort(t)
	cfg := &config.Manager{Data: config.AppConfig{Port: port, AppID: "CUSTOM_SERIAL_123"}}
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
	respInit, err := s.app.Test(reqInit)
	if err != nil {
		t.Fatalf("GET /odoo/ failed: %v", err)
	}
	if respInit.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 from /odoo/, got %d", respInit.StatusCode)
	}

	// 2. Connect via /odoo/connect
	reqConnect := httptest.NewRequest("GET", "/odoo/connect?db_url=http://127.0.0.1:8069&token=test-token&db_uuid=test-uuid", nil)
	respConnect, err := s.app.Test(reqConnect)
	if err != nil {
		t.Fatalf("GET /odoo/connect failed: %v", err)
	}
	if respConnect.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 from /odoo/connect, got %d", respConnect.StatusCode)
	}

	// 3. Test /odoo/ returns the custom serial
	reqStatus := httptest.NewRequest("GET", "/odoo/", nil)
	respStatus, err := s.app.Test(reqStatus)
	if err != nil {
		t.Fatalf("GET /odoo/ failed: %v", err)
	}
	bodyBytes, _ := io.ReadAll(respStatus.Body)
	if !strings.Contains(string(bodyBytes), serverSerial) {
		t.Errorf("Expected /odoo/ response to contain %q, got: %s", serverSerial, string(bodyBytes))
	}
}
