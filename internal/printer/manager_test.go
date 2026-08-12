package printer

import (
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"epos-proxy/internal/testutil"

	"github.com/google/gousb"
)

func TestNewManager(t *testing.T) {
	mgr := NewManager()
	testutil.ExpectedNotNil(t, mgr)
	testutil.ExpectedNotNil(t, mgr.printers)
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
		testutil.ExpectedNoError(t, err)
	}

	// 101st enqueue must return ErrQueueFull
	err := p.Enqueue(func(p *Printer) JobResult {
		return JobResult{OK: true}
	}, nil)

	testutil.ExpectedTrue(t, errors.Is(err, ErrQueueFull))
}

func TestPrinter_IdToString(t *testing.T) {
	// LAN printer
	pLAN := &Printer{
		connectionType: ConnKindLAN,
		lanIP:          "192.168.1.50",
	}
	testutil.ExpectedEqual(t, pLAN.idToString(), "LAN:192.168.1.50")

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
	testutil.ExpectedNotEqual(t, str, "")
	testutil.ExpectedNotEqual(t, str, "USB:unknown")

	// USB printer without ID
	pUSBEmpty := &Printer{
		connectionType: ConnKindUSB,
		id:             nil,
	}
	testutil.ExpectedEqual(t, pUSBEmpty.idToString(), "USB:unknown")
}

func TestManager_LANPrinterIntegration(t *testing.T) {
	var receivedData []byte
	var mu sync.Mutex
	done := make(chan struct{})

	// Start a mock TCP printer listener on 127.0.0.1:9100
	_, _, err := testutil.StartMockTCPServer(t, LANPort, func(conn net.Conn) {
		buf := make([]byte, 1024)
		n, _ := io.ReadAtLeast(conn, buf, 1)
		mu.Lock()
		receivedData = append(receivedData, buf[:n]...)
		mu.Unlock()
		select {
		case <-done:
		default:
			close(done)
		}
	})
	if err != nil {
		t.Skipf("Cannot bind port %d for mock printer integration test: %v", LANPort, err)
	}

	mgr := NewManager()
	printerID := EncodeLANPrinterID("127.0.0.1")

	testPayload := []byte("TEST PRINT DATA FOR LAN")
	replyChan, err := mgr.WriteAsync(printerID, testPayload)
	testutil.ExpectedNoError(t, err)

	select {
	case res := <-replyChan:
		testutil.ExpectedTrue(t, res.OK)
		testutil.ExpectedNoError(t, res.Err)
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for print job reply")
	}

	select {
	case <-done:
		mu.Lock()
		testutil.ExpectedBytesEqual(t, receivedData, testPayload)
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
	testutil.ExpectedEqual(t, got, "1.2.3.4")
}
