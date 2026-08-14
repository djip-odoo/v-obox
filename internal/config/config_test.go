package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewManagerWithPath_DefaultsAndSave(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "sub", "config.json")

	cm := NewManagerWithPath(configPath)
	if cm.Path() != configPath {
		t.Fatalf("Expected path %s, got %s", configPath, cm.Path())
	}

	if cm.Data.Port != 0 {
		t.Fatalf("Expected default port 0, got %d", cm.Data.Port)
	}

	if len(cm.Data.LANPrinters) != 0 {
		t.Fatalf("Expected empty LAN printers, got %v", cm.Data.LANPrinters)
	}

	// Save to disk
	cm.Data.Port = 4545
	if err := cm.Save(); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Reload in a new manager
	cm2 := NewManagerWithPath(configPath)
	if err := cm2.Load(); err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cm2.Data.Port != 4545 {
		t.Fatalf("Expected port 4545, got %d", cm2.Data.Port)
	}
}

func TestLoad_NonExistentFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "non_existent_config.json")

	cm := NewManagerWithPath(configPath)
	if err := cm.Load(); err != nil {
		t.Fatalf("Loading non-existent config should not error, got: %v", err)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "bad_config.json")

	if err := os.WriteFile(configPath, []byte("invalid-json{"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	cm := NewManagerWithPath(configPath)
	if err := cm.Load(); err == nil {
		t.Fatal("Expected error loading invalid JSON, got nil")
	}
}

func TestNewManager_UserDir(t *testing.T) {
	cm, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager() failed: %v", err)
	}
	if cm == nil || cm.Path() == "" {
		t.Fatal("Expected non-empty manager and path")
	}
}
