package printer

import (
	"errors"
	"testing"

	"epos-proxy/internal/testutil"
)

func TestEncodePrinterID(t *testing.T) {
	// Case 1: Serial provided -> encodes only s:<serial>
	pWithSerial := &LibUsbPrinter{
		Serial: "SN123456",
		VidPid: "04B8:0202",
		Path:   "1.2.3",
	}
	encoded, err := encodePrinterID(pWithSerial)
	testutil.ExpectedNoError(t, err)

	decoded, err := decodePrinterID(encoded)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, decoded.Serial, "SN123456")
	testutil.ExpectedEqual(t, decoded.VidPid, "")
	testutil.ExpectedEqual(t, decoded.Path, "")

	// Case 2: No serial, but VidPid and Path provided
	pNoSerial := &LibUsbPrinter{
		VidPid: "04B8:0202",
		Path:   "1.2.3",
	}
	encoded2, err := encodePrinterID(pNoSerial)
	testutil.ExpectedNoError(t, err)

	decoded2, err := decodePrinterID(encoded2)
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedEqual(t, decoded2.VidPid, "04B8:0202")
	testutil.ExpectedEqual(t, decoded2.Path, "1.2.3")

	// Case 3: Completely empty LibUsbPrinter -> should return error
	pEmpty := &LibUsbPrinter{}
	_, err = encodePrinterID(pEmpty)
	testutil.ExpectedError(t, err)
}

func TestDecodePrinterID_Invalid(t *testing.T) {
	// Invalid base64
	_, err := decodePrinterID("not-valid-base64!!!")
	testutil.ExpectedTrue(t, errors.Is(err, ErrInvalidPrinterID))

	// Valid base64 but empty payload
	_, err = decodePrinterID("")
	testutil.ExpectedTrue(t, errors.Is(err, ErrInvalidPrinterID))
}

func TestLANPrinterID_Roundtrip(t *testing.T) {
	ip := "192.168.1.150"
	encoded := EncodeLANPrinterID(ip)

	decoded, ok := DecodeLANPrinterID(encoded)
	testutil.ExpectedTrue(t, ok)
	testutil.ExpectedEqual(t, decoded, ip)
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
			testutil.ExpectedFalse(t, ok)
		})
	}
}

func TestID_String(t *testing.T) {
	// 1. nil ID
	var nilID *ID
	testutil.ExpectedEqual(t, nilID.String(), "unknown")

	// 2. Serial only
	idSerial := &ID{Serial: "SN123456"}
	testutil.ExpectedEqual(t, idSerial.String(), "serial=SN123456")

	// 3. VidPid and Path
	idVidPidPath := &ID{VidPid: "04B8:0202", Path: "1.2.3"}
	testutil.ExpectedEqual(t, idVidPidPath.String(), "vid:pid=04B8:0202, path=1.2.3")

	// 4. VidPid only
	idVidPid := &ID{VidPid: "04B8:0202"}
	testutil.ExpectedEqual(t, idVidPid.String(), "vid:pid=04B8:0202")

	// 5. Path only
	idPath := &ID{Path: "1.2.3"}
	testutil.ExpectedEqual(t, idPath.String(), "path=1.2.3")

	// 6. Serial, VidPid, and Path
	idAll := &ID{Serial: "SN123", VidPid: "04B8:0202", Path: "1.2"}
	testutil.ExpectedEqual(t, idAll.String(), "serial=SN123, vid:pid=04B8:0202, path=1.2")

	// 7. Empty ID struct
	idEmpty := &ID{}
	testutil.ExpectedEqual(t, idEmpty.String(), "unknown")
}
