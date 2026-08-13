package config

import (
	"fmt"
	"log"
	"net"
)

const (
	PortRangeStart = 4545
	PortRangeEnd   = 4555
)

func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func findAvailablePort(start, end int) (int, error) {
	for p := start; p <= end; p++ {
		if isPortAvailable(p) {
			return p, nil
		}
	}

	listener, err := net.Listen("tcp", ":0")
	if err == nil {
		port := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		return port, nil
	}

	return 0, err
}

// ResolvePort checks if configured port is available, or searches available range and saves it.
func (cm *Manager) ResolvePort() (int, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.Data.Port > 0 && isPortAvailable(cm.Data.Port) {
		return cm.Data.Port, nil
	}

	port, err := findAvailablePort(PortRangeStart, PortRangeEnd)
	if err != nil {
		return 0, err
	}

	cm.Data.Port = port
	if err := cm.saveLocked(); err != nil {
		log.Printf("[config] warning: could not save: %v\n", err)
	}
	return port, nil
}

// GetPort returns the currently configured port.
func (cm *Manager) GetPort() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Port
}

// SetPort sets and saves the port configuration.
func (cm *Manager) SetPort(port int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.Data.Port = port
	return cm.saveLocked()
}
