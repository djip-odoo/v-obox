package config

func (cm *Manager) UpdateFirewallPreference(accepted bool) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.Data.FirewallEnabled = accepted
	if accepted {
		cm.Data.OldPort = cm.Data.Port
	}
	return cm.saveLocked()
}

func (cm *Manager) IsFirewallEnabled() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.FirewallEnabled
}
