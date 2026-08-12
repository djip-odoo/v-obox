package printer

import (
	"errors"
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

	// Multi-interface device where 2nd interface is a Printer with Bulk OUT
	descMultiInterface := &gousb.DeviceDesc{
		Vendor:  0x04B8,
		Product: 0x0202,
		Configs: map[int]gousb.ConfigDesc{
			1: {
				Number: 1,
				Interfaces: []gousb.InterfaceDesc{
					{
						Number: 0,
						AltSettings: []gousb.InterfaceSetting{
							{
								Class: gousb.ClassHID,
							},
						},
					},
					{
						Number: 1,
						AltSettings: []gousb.InterfaceSetting{
							{
								Class: gousb.ClassPrinter,
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

	epInfoMulti, okMulti := findPrinterEndpoint(descMultiInterface)
	testutil.ExpectedTrue(t, okMulti)
	testutil.ExpectedEqual(t, epInfoMulti.iFace, 1)
	testutil.ExpectedEqual(t, epInfoMulti.outEndpoint, 2)
}

func TestFingerprintKey(t *testing.T) {
	desc1 := &gousb.DeviceDesc{
		Bus:     1,
		Address: 4,
		Vendor:  0x04B8,
		Product: 0x0202,
		Path:    []int{1, 2},
	}

	desc2 := &gousb.DeviceDesc{
		Bus:     1,
		Address: 5,
		Vendor:  0x04B8,
		Product: 0x0202,
		Path:    []int{1, 2},
	}

	key1 := fingerprintKey(desc1)
	key2 := fingerprintKey(desc2)

	testutil.ExpectedTrue(t, key1 != "")
	testutil.ExpectedTrue(t, key2 != "")
	testutil.ExpectedEqual(t, key1, fingerprintKey(desc1))
	testutil.ExpectedNotEqual(t, key1, key2)
}

func TestListUSBPrinters(t *testing.T) {
	// Call ListUSBPrinters directly - should not panic
	printers, err := ListUSBPrinters()
	if err != nil {
		t.Logf("ListUSBPrinters returned error (expected if libusb restricted): %v", err)
		return
	}
	testutil.ExpectedNotNil(t, printers)

	// Second invocation should use cache
	cached, err2 := ListUSBPrinters()
	testutil.ExpectedNoError(t, err2)
	testutil.ExpectedNotNil(t, cached)
}

func TestListUSBPrinters_WithMockOpenDevices(t *testing.T) {
	mockPrinters := []*gousb.DeviceDesc{
		{
			Bus:     1,
			Address: 2,
			Vendor:  0x04B8,
			Product: 0x0202,
			Path:    []int{1, 2},
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
										1: {Number: 1, Direction: gousb.EndpointDirectionOut, TransferType: gousb.TransferTypeBulk},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Bus:     1,
			Address: 3,
			Vendor:  0x0A5F,
			Product: 0x0187,
			Path:    []int{1, 3},
			Configs: map[int]gousb.ConfigDesc{
				1: {
					Number: 1,
					Interfaces: []gousb.InterfaceDesc{
						{
							Number: 0,
							AltSettings: []gousb.InterfaceSetting{
								{
									Class: gousb.ClassVendorSpec,
									Endpoints: map[gousb.EndpointAddress]gousb.EndpointDesc{
										2: {Number: 2, Direction: gousb.EndpointDirectionOut, TransferType: gousb.TransferTypeBulk},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			// Mass storage device (should be ignored)
			Bus:     1,
			Address: 4,
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
								},
							},
						},
					},
				},
			},
		},
	}

	oldOpenDevices := openDevices
	defer func() {
		openDevices = oldOpenDevices
		usbCache.Update(nil, nil, nil)
	}()

	openDevices = func(ctx *gousb.Context, fn func(desc *gousb.DeviceDesc) bool) ([]*gousb.Device, error) {
		for _, d := range mockPrinters {
			_ = fn(d)
		}
		return nil, nil
	}

	res, err := ListUSBPrinters()
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedNotNil(t, res)

	// Second invocation should hit cache
	cachedRes, err := ListUSBPrinters()
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedNotNil(t, cachedRes)
}

func TestListUSBPrinters_OpenDevicesError(t *testing.T) {
	oldOpenDevices := openDevices
	defer func() {
		openDevices = oldOpenDevices
		usbCache.Update(nil, nil, nil)
	}()

	openDevices = func(ctx *gousb.Context, fn func(desc *gousb.DeviceDesc) bool) ([]*gousb.Device, error) {
		return nil, errors.New("simulated libusb error")
	}

	res, err := ListUSBPrinters()
	testutil.ExpectedError(t, err)
	testutil.ExpectedNil(t, res)
}

func TestListUSBPrinters_UnavailableDevice(t *testing.T) {
	mockPrinters := []*gousb.DeviceDesc{
		{
			Bus:     1,
			Address: 2,
			Vendor:  0x04B8,
			Product: 0x0202,
			Path:    []int{1, 2},
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
										1: {Number: 1, Direction: gousb.EndpointDirectionOut, TransferType: gousb.TransferTypeBulk},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	oldOpenDevices := openDevices
	defer func() {
		openDevices = oldOpenDevices
		usbCache.Update(nil, nil, nil)
	}()

	callCount := 0
	openDevices = func(ctx *gousb.Context, fn func(desc *gousb.DeviceDesc) bool) ([]*gousb.Device, error) {
		callCount++
		if callCount == 1 {
			// First scan call lists descriptors
			for _, d := range mockPrinters {
				_ = fn(d)
			}
			return nil, nil
		}
		// Second call (in GetPrinterInfo) fails opening device
		return nil, errors.New("device locked by another process")
	}

	res, err := ListUSBPrinters()
	testutil.ExpectedNoError(t, err)
	testutil.ExpectedNotNil(t, res)
	testutil.ExpectedLen(t, res.Unavailable, 1)
	testutil.ExpectedTrue(t, res.Unavailable[0].Name != "")
	testutil.ExpectedEqual(t, res.Unavailable[0].Error, "failed to open USB device for info retrieval: device locked by another process")
}

func TestGetPrinterFriendlyName(t *testing.T) {
	name := getPrinterFriendlyName("04B8", "0202")
	testutil.ExpectedTrue(t, name != "")
}
