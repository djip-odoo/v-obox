package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Manager coordinates thread-safe access to persistent application configuration.
type Manager struct {
	mu   sync.RWMutex
	path string
	Data AppConfig
}

// NewManager initializes the config manager using the default OS user config directory.
func NewManager() (*Manager, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("cannot locate user config dir: %w", err)
	}

	dir := filepath.Join(base, AppName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create config dir: %w", err)
	}

	return NewManagerWithPath(filepath.Join(dir, "config.json")), nil
}

// NewManagerWithPath initializes a config manager targeting a specific file path.
func NewManagerWithPath(path string) *Manager {
	return &Manager{
		path: path,
		Data: defaults(),
	}
}

// Load reads and parses configuration from the file system into memory.
func (cm *Manager) Load() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("config read error: %w", err)
	}

	if err := json.Unmarshal(data, &cm.Data); err != nil {
		return fmt.Errorf("config parse error: %w", err)
	}
	return nil
}

// Save writes current in-memory configuration to disk.
func (cm *Manager) Save() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.saveLocked()
}

// saveLocked must be called with cm.mu write lock held.
func (cm *Manager) saveLocked() error {
	if cm.path == "" {
		return nil
	}

	dir := filepath.Dir(cm.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("cannot create config dir: %w", err)
		}
	}

	data, err := json.MarshalIndent(cm.Data, "", "  ")
	if err != nil {
		return fmt.Errorf("config marshal error: %w", err)
	}
	if err := os.WriteFile(cm.path, data, 0644); err != nil {
		return fmt.Errorf("config write error: %w", err)
	}
	return nil
}

// Path returns the config file path.
func (cm *Manager) Path() string {
	return cm.path
}
