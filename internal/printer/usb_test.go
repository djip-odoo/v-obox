package printer

import (
	"testing"

	"epos-proxy/internal/testutil"

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
	testutil.ExpectedTrue(t, ok)
	testutil.ExpectedEqual(t, epNum, 2)

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
	testutil.ExpectedFalse(t, okInterrupt)

	// Case 3: Empty endpoints
	_, okEmpty := matchBulkOutEndpoint(gousb.InterfaceSetting{})
	testutil.ExpectedFalse(t, okEmpty)
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
	testutil.ExpectedTrue(t, ok)
	testutil.ExpectedEqual(t, epInfo.outEndpoint, 1)
	testutil.ExpectedEqual(t, epInfo.iFace, 0)
	testutil.ExpectedEqual(t, epInfo.config, 1)

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
	testutil.ExpectedTrue(t, okKnown)
	testutil.ExpectedEqual(t, epInfoKnown.outEndpoint, 2)

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
	testutil.ExpectedFalse(t, okNonPrinter)

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
	testutil.ExpectedFalse(t, okMissing)
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
	testutil.ExpectedTrue(t, key != "")
	testutil.ExpectedEqual(t, key, fingerprintKey(desc))
}
