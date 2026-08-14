package config

// AddLanEposPrinter adds a LAN printer IP if not already present and saves the config.
func (cm *Manager) AddLanEposPrinter(ip string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, existing := range cm.Data.LANPrinters {
		if existing == ip {
			return nil // Already exists
		}
	}
	cm.Data.LANPrinters = append(cm.Data.LANPrinters, ip)
	return cm.saveLocked()
}

// RemoveLANPrinter removes a LAN printer IP if found and saves the config.
func (cm *Manager) RemoveLANPrinter(ip string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, existing := range cm.Data.LANPrinters {
		if existing == ip {
			cm.Data.LANPrinters = append(cm.Data.LANPrinters[:i], cm.Data.LANPrinters[i+1:]...)
			return cm.saveLocked()
		}
	}
	return nil // Not found, nothing to remove
}

// GetLANPrinters returns a copy of the list of configured LAN printer IPs.
func (cm *Manager) GetLANPrinters() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.Data.LANPrinters == nil {
		return []string{}
	}
	// Return a copy to avoid races if caller modifies the slice
	result := make([]string, len(cm.Data.LANPrinters))
	copy(result, cm.Data.LANPrinters)
	return result
}
