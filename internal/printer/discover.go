package printer

import (
	"fmt"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"
)

// Device describes a discovered printer (USB or LAN).
type Device struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
	Type       Type   `json:"type"`
	IsLAN      bool   `json:"isLAN"`
	LANIp      string `json:"lanIp,omitempty"`
	Online     bool   `json:"online"`
}

// DiscoveryResult holds the full result of a printer scan across all transports.
type DiscoveryResult struct {
	Available   []Device          `json:"available"`
	Unavailable []UnavailableInfo `json:"unavailable"`
	Error       error             `json:"error,omitempty"`
}

// DiscoverAllPrinters queries both USB and configured LAN printers,
// acting as a single mediator for printer discovery across the app and webserver.
func DiscoverAllPrinters(cfg *config.Manager) DiscoveryResult {
	available := make([]Device, 0)
	unavailable := make([]UnavailableInfo, 0)
	var scanErr error

	// 1. USB printers
	usbPrinters, err := ListUSBPrinters()
	if err != nil {
		scanErr = err
		logger.Errorf("USB printer detection failed: %v", err)
	} else if usbPrinters != nil {
		logger.Debugf("Detected %d available USB printers", len(usbPrinters.Available))
		for _, info := range usbPrinters.Available {
			available = append(available, Device{
				Identifier: info.Id,
				Name:       info.Name,
				Type:       info.Type,
				IsLAN:      false,
				Online:     true,
			})
		}

		for _, info := range usbPrinters.Unavailable {
			unavailable = append(unavailable, info)
			logger.Warnf("USB printer unavailable: %s (%s)", info.Name, info.Error)
		}
	}

	// 2. LAN printers
	if cfg != nil {
		lanPrinters := ListLANPrinters(cfg)
		for _, lan := range lanPrinters {
			available = append(available, Device{
				Identifier: lan.Id,
				Name:       fmt.Sprintf("Network - %s", lan.IP),
				Type:       TypeReceipt,
				IsLAN:      true,
				LANIp:      lan.IP,
				Online:     true,
			})
		}
	}

	return DiscoveryResult{
		Available:   available,
		Unavailable: unavailable,
		Error:       scanErr,
	}
}
