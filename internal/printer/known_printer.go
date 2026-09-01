package printer

import (
	"strings"
)

// Some thermal printers do not expose the standard USB printer class (0x07)
// and instead use vendor-specific interfaces. These known VID:PID pairs are
// treated as printers even when printer-class detection fails.
var printerRegistry = map[string]Type{
	// Receipt printers
	"2aaf:6015": TypeReceipt, // Essae thermal
	"04b8:0e32": TypeReceipt, // Epson thermal
	"04b8:0202": TypeReceipt, // Epson thermal
	"04b8:0203": TypeReceipt, // Epson thermal
	"04b8:0e27": TypeReceipt, // Epson TM-T83III
	"2d84:c7c8": TypeReceipt, // Zhuhai Poskey
	"4b43:3830": TypeReceipt, // Caysn
	"0483:5720": TypeReceipt, // STMicroelectronics

	// Label printers
	"0a5f:0187": TypeLabel, // Zebra ZD421
	"195f:0001": TypeLabel, // Godex G500
}

func isKnownPrinterVidPid(vidPid string) bool {
	_, ok := printerRegistry[strings.ToLower(vidPid)]
	return ok
}

func getPrinterType(vidPid string) Type {
	if printerType, ok := printerRegistry[strings.ToLower(vidPid)]; ok {
		return printerType
	}
	return TypeReceipt
}
