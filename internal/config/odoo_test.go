package config

import (
	"path/filepath"
	"testing"
)

func TestOdooConfig_SetAndPersist(t *testing.T) {
	tempDir := t.TempDir()

	cm := &Manager{
		path: filepath.Join(tempDir, "config.json"),
		Data: AppConfig{Port: 4545},
	}

	if cm.HasOdooCredentials() {
		t.Fatal("Expected HasOdooCredentials to be false initially")
	}

	err := cm.SetOdooCredentials("http://192.168.1.50:8069", "token-abc-123", "db-uuid-xyz")
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
	if cm.GetOdooDbUUID() != "db-uuid-xyz" {
		t.Fatalf("Expected dbUUID db-uuid-xyz, got %s", cm.GetOdooDbUUID())
	}

	// Reload in a new manager
	cm2 := &Manager{
		path: filepath.Join(tempDir, "config.json"),
		Data: AppConfig{Port: 4545},
	}

	if err := cm2.Load(); err != nil {
		t.Fatalf("Failed to reload: %v", err)
	}

	odooCfg := cm2.GetOdooConfig()
	if odooCfg.DbURL != "http://192.168.1.50:8069" ||
		odooCfg.Token != "token-abc-123" ||
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
}
