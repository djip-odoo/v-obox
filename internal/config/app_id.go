package config

import (
	"github.com/google/uuid"
)

// GetAppID returns the current application ID from config.
func (cm *Manager) GetAppID() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.AppID
}

// SetAppID sets and persists the application ID.
func (cm *Manager) SetAppID(id string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.Data.AppID = id
	return cm.saveLocked()
}

// EnsureAppID returns the existing application ID, or generates a new UUID v4,
// stores it in application storage, and returns it.
func (cm *Manager) EnsureAppID() (string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.Data.AppID != "" {
		return cm.Data.AppID, nil
	}

	cm.Data.AppID = uuid.New().String()
	if err := cm.saveLocked(); err != nil {
		return cm.Data.AppID, err
	}

	return cm.Data.AppID, nil
}
