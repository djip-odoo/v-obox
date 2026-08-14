package config

import (
	"path/filepath"
	"testing"
)

func TestOdooConfig_SetAndPersist(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	cm := NewManagerWithPath(configPath)

	if cm.HasOdooCredentials() {
		t.Fatal("Expected HasOdooCredentials to be false initially")
	}

	err := cm.SetOdooCredentials("http://192.168.1.50:8069", "token-abc-123", "ODO-12345", "db-uuid-xyz")
	if err != nil {
		t.Fatalf("SetOdooCredentials failed: %v", err)
	}

	if !cm.HasOdooCredentials() {
		t.Fatal("Expected HasOdooCredentials to be true")
	}

	if cm.GetOdooDbURL() != "http://192.168.1.50:8069" {
		t.Fatalf("Expected dbURL http://192.168.1.50:8069, got %s", cm.GetOdooDbURL())
	}
	if cm.GetOdooToken() != "token-abc-123" {
		t.Fatalf("Expected token token-abc-123, got %s", cm.GetOdooToken())
	}
	if cm.GetOdooSerial() != "ODO-12345" {
		t.Fatalf("Expected serial ODO-12345, got %s", cm.GetOdooSerial())
	}
	if cm.GetOdooDbUUID() != "db-uuid-xyz" {
		t.Fatalf("Expected dbUUID db-uuid-xyz, got %s", cm.GetOdooDbUUID())
	}

	// Reload in a new manager
	cm2 := NewManagerWithPath(configPath)
	if err := cm2.Load(); err != nil {
		t.Fatalf("Failed to reload: %v", err)
	}

	odooCfg := cm2.GetOdooConfig()
	if odooCfg.DbURL != "http://192.168.1.50:8069" ||
		odooCfg.Token != "token-abc-123" ||
		odooCfg.SerialNumber != "ODO-12345" ||
		odooCfg.DbUUID != "db-uuid-xyz" {
		t.Fatalf("Reloaded OdooConfig does not match expected: %+v", odooCfg)
	}

	// Clear Odoo config
	if err := cm2.ClearOdooConfig(); err != nil {
		t.Fatalf("ClearOdooConfig failed: %v", err)
	}
	if cm2.HasOdooCredentials() {
		t.Fatal("Expected HasOdooCredentials to be false after clear")
	}
	if cm2.GetOdooSerial() != "" {
		t.Fatalf("Expected empty serial after clear, got %s", cm2.GetOdooSerial())
	}
}
