package config

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestAppID_EnsureAndPersist(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	cm := NewManagerWithPath(configPath)

	// Initially empty
	if id := cm.GetAppID(); id != "" {
		t.Fatalf("Expected empty AppID initially, got %s", id)
	}

	// EnsureAppID should generate and save
	appID, err := cm.EnsureAppID()
	if err != nil {
		t.Fatalf("EnsureAppID failed: %v", err)
	}
	if appID == "" {
		t.Fatal("Expected non-empty AppID")
	}

	// Validate it is a valid UUID
	if _, err := uuid.Parse(appID); err != nil {
		t.Fatalf("Expected valid UUID format, got %s: %v", appID, err)
	}

	// Calling EnsureAppID again should return the identical AppID
	appID2, err := cm.EnsureAppID()
	if err != nil {
		t.Fatalf("Second EnsureAppID failed: %v", err)
	}
	if appID2 != appID {
		t.Fatalf("Expected same AppID %s, got %s", appID, appID2)
	}

	// Reload from disk in a new Manager
	cm2 := NewManagerWithPath(configPath)
	if err := cm2.Load(); err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}
	if cm2.GetAppID() != appID {
		t.Fatalf("Expected reloaded AppID to match %s, got %s", appID, cm2.GetAppID())
	}
}

func TestAppID_SetCustom(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	cm := NewManagerWithPath(configPath)

	customID := "custom-machine-id-12345"
	if err := cm.SetAppID(customID); err != nil {
		t.Fatalf("SetAppID failed: %v", err)
	}

	if cm.GetAppID() != customID {
		t.Fatalf("Expected AppID %s, got %s", customID, cm.GetAppID())
	}

	// EnsureAppID should preserve custom ID
	ensured, err := cm.EnsureAppID()
	if err != nil {
		t.Fatalf("EnsureAppID failed: %v", err)
	}
	if ensured != customID {
		t.Fatalf("Expected ensured AppID to be %s, got %s", customID, ensured)
	}
}
