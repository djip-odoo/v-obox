package config

type OdooConfig struct {
	DbURL  string `json:"db_url,omitempty"`
	Token  string `json:"token,omitempty"`
	DbUUID string `json:"db_uuid,omitempty"`
}

func (cm *Manager) GetOdooConfig() OdooConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Odoo
}

func (cm *Manager) SetOdooConfig(cfg OdooConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.Data.Odoo = cfg
	return cm.saveLocked()
}

func (cm *Manager) SetOdooCredentials(dbURL, token, dbUUID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if dbURL != "" {
		cm.Data.Odoo.DbURL = dbURL
	}
	if token != "" {
		cm.Data.Odoo.Token = token
	}
	if dbUUID != "" {
		cm.Data.Odoo.DbUUID = dbUUID
	}

	return cm.saveLocked()
}

func (cm *Manager) ClearOdooConfig() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.Data.Odoo = OdooConfig{}
	return cm.saveLocked()
}

func (cm *Manager) GetOdooDbURL() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Odoo.DbURL
}

func (cm *Manager) GetOdooToken() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Odoo.Token
}

func (cm *Manager) GetOdooDbUUID() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Odoo.DbUUID
}

func (cm *Manager) HasOdooCredentials() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.Odoo.DbURL != "" && cm.Data.Odoo.Token != ""
}
