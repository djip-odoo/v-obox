package config

import (
	"net"
	"path/filepath"
	"testing"
)

func TestPortResolutionAndSet(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	cm := NewManagerWithPath(configPath)

	port, err := cm.ResolvePort()
	if err != nil {
		t.Fatalf("ResolvePort failed: %v", err)
	}
	if port < PortRangeStart || port > PortRangeEnd {
		t.Fatalf("Expected port in range [%d, %d], got %d", PortRangeStart, PortRangeEnd, port)
	}

	if cm.GetPort() != port {
		t.Fatalf("Expected GetPort() == %d, got %d", port, cm.GetPort())
	}

	// Test SetPort
	if err := cm.SetPort(4550); err != nil {
		t.Fatalf("SetPort failed: %v", err)
	}
	if cm.GetPort() != 4550 {
		t.Fatalf("Expected port 4550, got %d", cm.GetPort())
	}
}

func TestResolvePort_OccupiedPort(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	cm := NewManagerWithPath(configPath)

	// Occupy PortRangeStart
	ln, err := net.Listen("tcp", "127.0.0.1:4545")
	if err == nil {
		defer ln.Close()
	}

	port, err := cm.ResolvePort()
	if err != nil {
		t.Fatalf("ResolvePort failed: %v", err)
	}
	if ln != nil && port == 4545 {
		t.Fatalf("ResolvePort should not have returned occupied port 4545")
	}
}
