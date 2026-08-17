package config

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const charsetAlphanumericUpper = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
const defaultAppIDLength = 10

func GenerateDefaultAppID() (string, error) {
	result := make([]byte, defaultAppIDLength)
	max := big.NewInt(int64(len(charsetAlphanumericUpper)))

	for i := range result {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate random app ID: %w", err)
		}
		result[i] = charsetAlphanumericUpper[n.Int64()]
	}

	return string(result), nil
}

func (cm *Manager) GetAppID() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.AppID
}

func (cm *Manager) EnsureAppID() (string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.Data.AppID != "" {
		return cm.Data.AppID, nil
	}

	appID, err := GenerateDefaultAppID()
	if err != nil {
		return "", err
	}

	cm.Data.AppID = appID
	if err := cm.saveLocked(); err != nil {
		return "", err
	}

	return appID, nil
}
