package printer

import (
	"sync"
	"testing"
)

func TestBuildSnapshot(t *testing.T) {
	// Keys in different order should yield identical snapshot
	keys1 := []string{"dev-B", "dev-A", "dev-C"}
	keys2 := []string{"dev-C", "dev-B", "dev-A"}

	snap1 := buildSnapshot(keys1)
	snap2 := buildSnapshot(keys2)

	if snap1 != snap2 {
		t.Errorf("Expected identical snapshots, got %q and %q", snap1, snap2)
	}

	expected := "dev-A|dev-B|dev-C"
	if snap1 != expected {
		t.Errorf("Expected snapshot %q, got %q", expected, snap1)
	}

	// Empty keys
	if got := buildSnapshot([]string{}); got != "" {
		t.Errorf("Expected empty snapshot for empty keys, got: %q", got)
	}
}

func TestPrinterCache_Lifecycle(t *testing.T) {
	cache := &printerCache{}

	keys := []string{"dev1", "dev2"}
	if !cache.HasChanged(keys) {
		t.Error("Expected HasChanged to be true on uninitialized cache")
	}

	available := []Info{
		{Id: "p1", Name: "Printer 1", Type: TypeReceipt},
	}
	unavailable := []UnavailableInfo{
		{Name: "Printer Bad", Error: "permission denied"},
	}

	cache.Update(keys, available, unavailable)

	// Now cache has been updated with keys -> HasChanged should be false
	if cache.HasChanged(keys) {
		t.Error("Expected HasChanged to be false after Update with same keys")
	}

	// HasUnavailable should be true
	if !cache.HasUnavailable() {
		t.Error("Expected HasUnavailable to be true")
	}

	// Get should return copy of available printers
	printers := cache.Get()
	if len(printers) != 1 || printers[0].Id != "p1" {
		t.Errorf("Unexpected cached printers: %+v", printers)
	}

	// Mutating returned slice should not mutate internal cache
	printers[0].Name = "MUTATED"
	if cache.Get()[0].Name == "MUTATED" {
		t.Error("cache.Get() did not return a defensive copy")
	}

	// Update with new keys and no unavailable printers
	newKeys := []string{"dev1", "dev2", "dev3"}
	cache.Update(newKeys, available, nil)

	if cache.HasUnavailable() {
		t.Error("Expected HasUnavailable to be false after clearing unavailable")
	}
	if cache.HasChanged(newKeys) {
		t.Error("Expected HasChanged to be false for updated keys")
	}
}

func TestPrinterCache_Concurrency(t *testing.T) {
	cache := &printerCache{}
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			keys := []string{"dev1", "dev2"}
			if id%2 == 0 {
				cache.Update(keys, []Info{{Id: "p1"}}, nil)
			} else {
				_ = cache.HasChanged(keys)
				_ = cache.Get()
				_ = cache.HasUnavailable()
			}
		}(i)
	}

	wg.Wait()
}
