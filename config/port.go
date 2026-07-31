package config

import (
	"fmt"
	"log"
	"net"
	"runtime"
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

func (cm *Manager) GetPort() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.Data.Port
}

func (cm *Manager) GetOldPort() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.Data.OldPort
}

func (cm *Manager) CheckPortChange() error {
	if runtime.GOOS != "linux" {
		return nil
	}

	if cm.IsFirewallEnabled() && cm.GetOldPort() != cm.GetPort() {
		if err := cm.UpdateFirewallPreference(false); err != nil {
			log.Printf("[config] warning: could not update firewall preference: %v\n", err)
			return err
		}
	}
	return nil
}
