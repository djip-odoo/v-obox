package printer

import (
	"path/filepath"
	"testing"

	"epos-proxy/internal/config"
	"epos-proxy/internal/testutil"
)

func TestDiscoverAllPrinters_NilConfig(t *testing.T) {
	result := DiscoverAllPrinters(nil)
	testutil.ExpectedNotNil(t, result.Available)
	testutil.ExpectedNotNil(t, result.Unavailable)
}

func TestDiscoverAllPrinters_WithLANPrinters(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &config.Manager{
		Data: config.AppConfig{
			LANPrinters: []string{"192.168.1.50", "192.168.1.51"},
		},
	}
	_ = cfg.Save()
	_ = filepath.Join(tempDir, "config.json")

	result := DiscoverAllPrinters(cfg)
	testutil.ExpectedNotNil(t, result.Available)

	found50 := false
	found51 := false
	for _, p := range result.Available {
		if p.LANIp == "192.168.1.50" {
			found50 = true
			testutil.ExpectedTrue(t, p.IsLAN)
			testutil.ExpectedEqual(t, p.Name, "Network - 192.168.1.50")
		}
		if p.LANIp == "192.168.1.51" {
			found51 = true
			testutil.ExpectedTrue(t, p.IsLAN)
		}
	}
	testutil.ExpectedTrue(t, found50, "Expected LAN printer 192.168.1.50 in discovered printers")
	testutil.ExpectedTrue(t, found51, "Expected LAN printer 192.168.1.51 in discovered printers")
}
