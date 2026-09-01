//go:build !android

package printer

import (
	"fmt"
	"strings"

	"github.com/google/gousb"
)

func isKnownPrinter(desc *gousb.DeviceDesc) bool {
	vidPid := strings.ToLower(
		fmt.Sprintf("%04x:%04x", uint16(desc.Vendor), uint16(desc.Product)),
	)
	return isKnownPrinterVidPid(vidPid)
}
