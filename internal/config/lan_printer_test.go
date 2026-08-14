package config

import (
	"path/filepath"
	"testing"
)

func TestLANPrinterManagement(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	cm := NewManagerWithPath(configPath)

	// Initially empty
	printers := cm.GetLANPrinters()
	if len(printers) != 0 {
		t.Fatalf("Expected 0 LAN printers, got %d", len(printers))
	}

	// Add printer 1
	if err := cm.AddLanEposPrinter("192.168.1.100"); err != nil {
		t.Fatalf("AddLanEposPrinter failed: %v", err)
	}

	// Add printer 2
	if err := cm.AddLanEposPrinter("192.168.1.101"); err != nil {
		t.Fatalf("AddLanEposPrinter failed: %v", err)
	}

	// Duplicate addition should be idempotent
	if err := cm.AddLanEposPrinter("192.168.1.100"); err != nil {
		t.Fatalf("AddLanEposPrinter duplicate failed: %v", err)
	}

	printers = cm.GetLANPrinters()
	if len(printers) != 2 {
		t.Fatalf("Expected 2 LAN printers, got %d", len(printers))
	}

	// Test persistence by reloading in new manager
	cm2 := NewManagerWithPath(configPath)
	if err := cm2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(cm2.GetLANPrinters()) != 2 {
		t.Fatalf("Expected 2 LAN printers after reload, got %d", len(cm2.GetLANPrinters()))
	}

	// Remove printer 1
	if err := cm.RemoveLANPrinter("192.168.1.100"); err != nil {
		t.Fatalf("RemoveLANPrinter failed: %v", err)
	}
	printers = cm.GetLANPrinters()
	if len(printers) != 1 || printers[0] != "192.168.1.101" {
		t.Fatalf("Expected only 192.168.1.101, got %v", printers)
	}

	// Removing non-existent printer should succeed gracefully
	if err := cm.RemoveLANPrinter("192.168.1.999"); err != nil {
		t.Fatalf("Remove non-existent printer failed: %v", err)
	}
}
