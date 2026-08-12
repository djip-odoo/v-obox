package printer

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/gousb"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager()
	if mgr == nil {
		t.Fatal("Expected non-nil Manager")
	}
	if mgr.printers == nil {
		t.Fatal("Expected initialized printers map")
	}
}

func TestPrinter_QueueFull(t *testing.T) {
	p := &Printer{
		connectionType: ConnKindLAN,
		lanIP:          "127.0.0.1",
		jobs:           make(chan Job, QueueSize),
	}
	// Note: We do not start p.loop() here so we can fill the queue deterministically

	// Fill the queue to QueueSize (100)
	for i := 0; i < QueueSize; i++ {
		err := p.Enqueue(func(p *Printer) JobResult {
			return JobResult{OK: true}
		}, nil)
		if err != nil {
			t.Fatalf("Enqueue failed on item %d: %v", i, err)
		}
	}

	// 101st enqueue must return ErrQueueFull
	err := p.Enqueue(func(p *Printer) JobResult {
		return JobResult{OK: true}
	}, nil)

	if !errors.Is(err, ErrQueueFull) {
		t.Errorf("Expected ErrQueueFull, got: %v", err)
	}
}

func TestPrinter_IdToString(t *testing.T) {
	// LAN printer
	pLAN := &Printer{
		connectionType: ConnKindLAN,
		lanIP:          "192.168.1.50",
	}
	if pLAN.idToString() != "LAN:192.168.1.50" {
		t.Errorf("Unexpected LAN idToString: %s", pLAN.idToString())
	}

	// USB printer with ID
	pUSB := &Printer{
		connectionType: ConnKindUSB,
		id: &ID{
			Serial: "SER123",
			VidPid: "04B8:0202",
			Path:   "1.2",
		},
	}
	str := pUSB.idToString()
	if str == "" || str == "USB:unknown" {
		t.Errorf("Unexpected USB idToString: %s", str)
	}

	// USB printer without ID
	pUSBEmpty := &Printer{
		connectionType: ConnKindUSB,
		id:             nil,
	}
	if pUSBEmpty.idToString() != "USB:unknown" {
		t.Errorf("Expected 'USB:unknown', got: %s", pUSBEmpty.idToString())
	}
}

func TestManager_LANPrinterIntegration(t *testing.T) {
	// Start a mock TCP printer listener on 127.0.0.1:9100 (if possible)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", LANPort))
	if err != nil {
		t.Skipf("Cannot bind port %d for mock printer integration test: %v", LANPort, err)
	}
	defer ln.Close()

	var receivedData []byte
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, _ := io.ReadAtLeast(conn, buf, 1)
		mu.Lock()
		receivedData = append(receivedData, buf[:n]...)
		mu.Unlock()
		close(done)
	}()

	mgr := NewManager()
	printerID := EncodeLANPrinterID("127.0.0.1")

	testPayload := []byte("TEST PRINT DATA FOR LAN")
	replyChan, err := mgr.WriteAsync(printerID, testPayload)
	if err != nil {
		t.Fatalf("WriteAsync failed: %v", err)
	}

	select {
	case res := <-replyChan:
		if !res.OK || res.Err != nil {
			t.Fatalf("Print job returned error: %v", res.Err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for print job reply")
	}

	select {
	case <-done:
		mu.Lock()
		if string(receivedData) != string(testPayload) {
			t.Errorf("Received data %q != expected %q", string(receivedData), string(testPayload))
		}
		mu.Unlock()
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for mock printer server to receive data")
	}
}

func TestPathToString(t *testing.T) {
	desc := &gousb.DeviceDesc{
		Bus:  1,
		Path: []int{2, 3, 4},
	}

	got := pathToString(desc)
	expected := "1.2.3.4"
	if got != expected {
		t.Errorf("pathToString() = %q, want %q", got, expected)
	}
}
