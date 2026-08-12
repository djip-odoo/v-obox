package printer

import (
	"testing"

	"github.com/google/gousb"
)

func TestMatchBulkOutEndpoint(t *testing.T) {
	// Case 1: Bulk OUT endpoint present
	altWithBulkOut := gousb.InterfaceSetting{
		Endpoints: map[gousb.EndpointAddress]gousb.EndpointDesc{
			gousb.EndpointAddress(1): {
				Number:       1,
				Direction:    gousb.EndpointDirectionIn,
				TransferType: gousb.TransferTypeBulk,
			},
			gousb.EndpointAddress(2): {
				Number:       2,
				Direction:    gousb.EndpointDirectionOut,
				TransferType: gousb.TransferTypeBulk,
			},
		},
	}

	epNum, ok := matchBulkOutEndpoint(altWithBulkOut)
	if !ok || epNum != 2 {
		t.Errorf("matchBulkOutEndpoint() = (%d, %v), want (2, true)", epNum, ok)
	}

	// Case 2: Only Interrupt OUT (not Bulk)
	altInterrupt := gousb.InterfaceSetting{
		Endpoints: map[gousb.EndpointAddress]gousb.EndpointDesc{
			gousb.EndpointAddress(3): {
				Number:       3,
				Direction:    gousb.EndpointDirectionOut,
				TransferType: gousb.TransferTypeInterrupt,
			},
		},
	}

	_, okInterrupt := matchBulkOutEndpoint(altInterrupt)
	if okInterrupt {
		t.Error("Expected matchBulkOutEndpoint to return false for interrupt transfer")
	}

	// Case 3: Empty endpoints
	_, okEmpty := matchBulkOutEndpoint(gousb.InterfaceSetting{})
	if okEmpty {
		t.Error("Expected matchBulkOutEndpoint to return false for empty endpoints")
	}
}

func TestFindPrinterEndpoint(t *testing.T) {
	// Standard USB printer class device (Class 7) with Bulk OUT endpoint
	descPrinterClass := &gousb.DeviceDesc{
		Vendor:  0x04B8,
		Product: 0x0001,
		Configs: map[int]gousb.ConfigDesc{
			1: {
				Number: 1,
				Interfaces: []gousb.InterfaceDesc{
					{
						Number: 0,
						AltSettings: []gousb.InterfaceSetting{
							{
								Number:    0,
								Alternate: 0,
								Class:     gousb.ClassPrinter,
								Endpoints: map[gousb.EndpointAddress]gousb.EndpointDesc{
									gousb.EndpointAddress(1): {
										Number:       1,
										Direction:    gousb.EndpointDirectionOut,
										TransferType: gousb.TransferTypeBulk,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	epInfo, ok := findPrinterEndpoint(descPrinterClass)
	if !ok {
		t.Fatal("Expected findPrinterEndpoint to find printer class endpoint")
	}
	if epInfo.outEndpoint != 1 || epInfo.iFace != 0 || epInfo.config != 1 {
		t.Errorf("Unexpected EndpointInfo: %+v", epInfo)
	}

	// Known printer (e.g. 0a5f:0187 Zebra) with vendor-specific class (Class 0xFF)
	descKnownVendorClass := &gousb.DeviceDesc{
		Vendor:  0x0A5F,
		Product: 0x0187,
		Configs: map[int]gousb.ConfigDesc{
			1: {
				Number: 1,
				Interfaces: []gousb.InterfaceDesc{
					{
						Number: 0,
						AltSettings: []gousb.InterfaceSetting{
							{
								Number:    0,
								Alternate: 0,
								Class:     gousb.ClassVendorSpec,
								Endpoints: map[gousb.EndpointAddress]gousb.EndpointDesc{
									gousb.EndpointAddress(2): {
										Number:       2,
										Direction:    gousb.EndpointDirectionOut,
										TransferType: gousb.TransferTypeBulk,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	epInfoKnown, okKnown := findPrinterEndpoint(descKnownVendorClass)
	if !okKnown {
		t.Fatal("Expected findPrinterEndpoint to identify known vendor-spec printer")
	}
	if epInfoKnown.outEndpoint != 2 {
		t.Errorf("Expected endpoint 2, got %d", epInfoKnown.outEndpoint)
	}

	// Non-printer device (e.g. Mass Storage Class 8, unknown VID/PID)
	descNonPrinter := &gousb.DeviceDesc{
		Vendor:  0x1234,
		Product: 0x5678,
		Configs: map[int]gousb.ConfigDesc{
			1: {
				Number: 1,
				Interfaces: []gousb.InterfaceDesc{
					{
						Number: 0,
						AltSettings: []gousb.InterfaceSetting{
							{
								Class: gousb.ClassMassStorage,
								Endpoints: map[gousb.EndpointAddress]gousb.EndpointDesc{
									gousb.EndpointAddress(1): {
										Number:       1,
										Direction:    gousb.EndpointDirectionOut,
										TransferType: gousb.TransferTypeBulk,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, okNonPrinter := findPrinterEndpoint(descNonPrinter)
	if okNonPrinter {
		t.Error("Expected findPrinterEndpoint to reject non-printer device")
	}

	// Printer class device missing Bulk OUT endpoint
	descMissingBulk := &gousb.DeviceDesc{
		Vendor:  0x04B8,
		Product: 0x0001,
		Configs: map[int]gousb.ConfigDesc{
			1: {
				Number: 1,
				Interfaces: []gousb.InterfaceDesc{
					{
						Number: 0,
						AltSettings: []gousb.InterfaceSetting{
							{
								Class: gousb.ClassPrinter,
								Endpoints: map[gousb.EndpointAddress]gousb.EndpointDesc{
									gousb.EndpointAddress(1): {
										Number:       1,
										Direction:    gousb.EndpointDirectionIn,
										TransferType: gousb.TransferTypeBulk,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, okMissing := findPrinterEndpoint(descMissingBulk)
	if okMissing {
		t.Error("Expected findPrinterEndpoint to return false when bulk OUT is missing")
	}
}

func TestFingerprintKey(t *testing.T) {
	desc := &gousb.DeviceDesc{
		Bus:     1,
		Address: 4,
		Vendor:  0x04B8,
		Product: 0x0202,
		Path:    []int{1, 2},
	}

	key := fingerprintKey(desc)
	if key == "" || key != fingerprintKey(desc) {
		t.Errorf("fingerprintKey() mismatch: %q", key)
	}
}
