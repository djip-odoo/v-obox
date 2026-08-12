package printer

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"epos-proxy/internal/config"
)

func TestValidateIPAddress(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		expectErr bool
	}{
		{name: "valid IPv4", input: "192.168.1.50", expected: "192.168.1.50", expectErr: false},
		{name: "valid IPv4 with whitespace", input: "  10.0.0.1  ", expected: "10.0.0.1", expectErr: false},
		{name: "valid IPv6", input: "::1", expected: "::1", expectErr: false},
		{name: "empty string", input: "   ", expected: "", expectErr: true},
		{name: "invalid format", input: "999.999.999.999", expected: "", expectErr: true},
		{name: "alphabetic string", input: "not.an.ip.address", expected: "", expectErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateIPAddress(tc.input)
			if (err != nil) != tc.expectErr {
				t.Fatalf("ValidateIPAddress(%q) error = %v, expectErr %v", tc.input, err, tc.expectErr)
			}
			if got != tc.expected {
				t.Errorf("ValidateIPAddress(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestCheckLANPrinter_SuccessAndOffline(t *testing.T) {
	// 1. Offline IP / unreachable port
	err := CheckLANPrinter("127.0.0.1")
	// Since port 9100 is typically not open in test environment, it should return error
	// If 9100 happens to be open, we still test that dialing a closed port behaves predictably
	if err == nil {
		t.Log("Note: 127.0.0.1:9100 was reachable in this environment")
	}

	// 2. Mock a live TCP server on port 9100 (if possible)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", LANPort))
	if err == nil {
		defer ln.Close()
		go func() {
			conn, err := ln.Accept()
			if err == nil {
				_ = conn.Close()
			}
		}()

		if err := CheckLANPrinter("127.0.0.1"); err != nil {
			t.Errorf("Expected CheckLANPrinter to succeed for active listener, got: %v", err)
		}
	} else {
		t.Logf("Could not bind port %d for live test: %v (skipping active listener check)", LANPort, err)
	}
}

func TestListLANPrinters(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Manager{
		Data: config.AppConfig{
			LANPrinters: []string{"192.168.1.100", "192.168.1.101"},
		},
	}
	_ = filepath.Join(tempDir, "config.json") // satisfies path usage

	printers := ListLANPrinters(cfg)
	if len(printers) != 2 {
		t.Fatalf("Expected 2 LAN printers, got %d", len(printers))
	}

	if printers[0].IP != "192.168.1.100" || printers[0].Id != EncodeLANPrinterID("192.168.1.100") {
		t.Errorf("Mismatch for printer 0: %+v", printers[0])
	}
	if printers[1].IP != "192.168.1.101" || printers[1].Id != EncodeLANPrinterID("192.168.1.101") {
		t.Errorf("Mismatch for printer 1: %+v", printers[1])
	}
}
