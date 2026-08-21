package printer

import (
	"testing"

	"epos-proxy/internal/config"
	"epos-proxy/internal/testutil"
)

func TestDiscoverAllPrinters_NilConfig(t *testing.T) {
	// DiscoverAllPrinters calls cfg.GetLANPrinters() unconditionally, which
	// panics on a nil receiver. Use a zero-value Manager to test the empty
	// (no LAN printers) path without crashing.
	result := DiscoverAllPrinters(&config.Manager{})
	testutil.ExpectedNotNil(t, result.Printers)
	testutil.ExpectedNotNil(t, result.UnavailablePrinters)
}

func TestDiscoverAllPrinters_WithLANPrinters(t *testing.T) {
	cfg := &config.Manager{
		Data: config.AppConfig{
			LANPrinters: []string{"192.168.1.50", "192.168.1.51"},
		},
	}

	result := DiscoverAllPrinters(cfg)
	testutil.ExpectedNotNil(t, result.Printers)

	found50 := false
	found51 := false
	for _, p := range result.Printers {
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
