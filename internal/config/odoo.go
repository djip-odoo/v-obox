package config

// GetOdooConfig returns a copy of the stored Odoo configuration.
func (cm *Manager) GetOdooConfig() OdooConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Odoo
}

// SetOdooConfig sets and persists the entire Odoo configuration.
func (cm *Manager) SetOdooConfig(cfg OdooConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.Data.Odoo = cfg
	return cm.saveLocked()
}

// SetOdooCredentials updates Odoo identifiers and persists them in application storage.
func (cm *Manager) SetOdooCredentials(dbURL, token, serial, dbUUID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if dbURL != "" {
		cm.Data.Odoo.DbURL = dbURL
	}
	if token != "" {
		cm.Data.Odoo.Token = token
	}
	if serial != "" {
		cm.Data.Odoo.SerialNumber = serial
	}
	if dbUUID != "" {
		cm.Data.Odoo.DbUUID = dbUUID
	}

	return cm.saveLocked()
}

// ClearOdooConfig removes stored Odoo identifiers and persists the change.
func (cm *Manager) ClearOdooConfig() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.Data.Odoo = OdooConfig{}
	return cm.saveLocked()
}

// GetOdooSerial returns the stored Odoo serial number.
func (cm *Manager) GetOdooSerial() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Odoo.SerialNumber
}

// GetOdooDbURL returns the stored Odoo database URL.
func (cm *Manager) GetOdooDbURL() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Odoo.DbURL
}

// GetOdooToken returns the stored Odoo token.
func (cm *Manager) GetOdooToken() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Odoo.Token
}

// GetOdooDbUUID returns the stored Odoo database UUID.
func (cm *Manager) GetOdooDbUUID() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Odoo.DbUUID
}

// HasOdooCredentials returns true if essential Odoo credentials (URL and token) are configured.
func (cm *Manager) HasOdooCredentials() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Odoo.DbURL != "" && cm.Data.Odoo.Token != ""
}
