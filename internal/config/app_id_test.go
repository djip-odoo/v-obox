package config

import (
	"path/filepath"
	"regexp"
	"testing"
)

func TestAppID_GenerateDefaultFormat(t *testing.T) {
	appID := GenerateDefaultAppID()
	if len(appID) != 10 {
		t.Fatalf("Expected 10 characters, got %d (%s)", len(appID), appID)
	}

	match, err := regexp.MatchString(`^[0-9A-Z]{10}$`, appID)
	if err != nil || !match {
		t.Fatalf("Expected 10 alphanumeric uppercase characters, got %s", appID)
	}
}

func TestAppID_EnsureAndPersist(t *testing.T) {
	tempDir := t.TempDir()

	cm := &Manager{
		path: filepath.Join(tempDir, "config.json"),
		Data: AppConfig{Port: 4545},
	}

	// EnsureAppID should generate and save 10-char uppercase ID
	appID, err := cm.EnsureAppID()
	if err != nil {
		t.Fatalf("EnsureAppID failed: %v", err)
	}
	if len(appID) != 10 {
		t.Fatalf("Expected 10-char AppID, got %d (%s)", len(appID), appID)
	}

	// Validate it is uppercase alphanumeric
	match, err := regexp.MatchString(`^[0-9A-Z]{10}$`, appID)
	if err != nil || !match {
		t.Fatalf("Expected 10 alphanumeric uppercase characters, got %s", appID)
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
	cm2 := &Manager{
		path: filepath.Join(tempDir, "config.json"),
		Data: AppConfig{Port: 4545},
	}

	if err := cm2.Load(); err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}
	if cm2.GetAppID() != appID {
		t.Fatalf("Expected reloaded AppID to match %s, got %s", appID, cm2.GetAppID())
	}
}
