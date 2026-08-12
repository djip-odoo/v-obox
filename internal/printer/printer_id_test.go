package printer

import (
	"errors"
	"testing"
)

func TestEncodePrinterID(t *testing.T) {
	// Case 1: Serial provided -> encodes only s:<serial>
	pWithSerial := &LibUsbPrinter{
		Serial: "SN123456",
		VidPid: "04B8:0202",
		Path:   "1.2.3",
	}
	encoded, err := encodePrinterID(pWithSerial)
	if err != nil {
		t.Fatalf("encodePrinterID failed: %v", err)
	}

	decoded, err := decodePrinterID(encoded)
	if err != nil {
		t.Fatalf("decodePrinterID failed: %v", err)
	}
	if decoded.Serial != "SN123456" {
		t.Errorf("Expected Serial SN123456, got: %s", decoded.Serial)
	}
	if decoded.VidPid != "" || decoded.Path != "" {
		t.Errorf("Expected VidPid and Path to be empty when serial was used, got: %+v", decoded)
	}

	// Case 2: No serial, but VidPid and Path provided
	pNoSerial := &LibUsbPrinter{
		VidPid: "04B8:0202",
		Path:   "1.2.3",
	}
	encoded2, err := encodePrinterID(pNoSerial)
	if err != nil {
		t.Fatalf("encodePrinterID failed: %v", err)
	}

	decoded2, err := decodePrinterID(encoded2)
	if err != nil {
		t.Fatalf("decodePrinterID failed: %v", err)
	}
	if decoded2.VidPid != "04B8:0202" || decoded2.Path != "1.2.3" {
		t.Errorf("Expected VidPid 04B8:0202 and Path 1.2.3, got: %+v", decoded2)
	}

	// Case 3: Completely empty LibUsbPrinter -> should return error
	pEmpty := &LibUsbPrinter{}
	_, err = encodePrinterID(pEmpty)
	if err == nil {
		t.Fatal("Expected error for empty LibUsbPrinter, got nil")
	}
}

func TestDecodePrinterID_Invalid(t *testing.T) {
	// Invalid base64
	_, err := decodePrinterID("not-valid-base64!!!")
	if !errors.Is(err, ErrInvalidPrinterID) {
		t.Errorf("Expected ErrInvalidPrinterID, got: %v", err)
	}

	// Valid base64 but empty payload
	_, err = decodePrinterID("")
	if !errors.Is(err, ErrInvalidPrinterID) {
		t.Errorf("Expected ErrInvalidPrinterID for empty string, got: %v", err)
	}
}

func TestLANPrinterID_Roundtrip(t *testing.T) {
	ip := "192.168.1.150"
	encoded := EncodeLANPrinterID(ip)

	decoded, ok := DecodeLANPrinterID(encoded)
	if !ok {
		t.Fatalf("DecodeLANPrinterID returned ok=false for %s", encoded)
	}
	if decoded != ip {
		t.Errorf("Expected IP %s, got %s", ip, decoded)
	}
}

func TestDecodeLANPrinterID_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"invalid base64", "!!!bad-base64"},
		{"empty string", ""},
		{"too short", "bA"},                    // decoded length < 3
		{"missing colon", "bHh4"},              // decoded "lxx"
		{"wrong prefix", "dToxOTIuMTY4LjEuMQ"}, // decoded "u:192.168.1.1"
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := DecodeLANPrinterID(tc.input)
			if ok {
				t.Errorf("DecodeLANPrinterID(%q) expected ok=false, got true", tc.input)
			}
		})
	}
}
