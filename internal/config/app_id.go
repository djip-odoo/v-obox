package config

import (
	"crypto/rand"
	"math/big"
)

const (
	charsetAlphanumeric = "0123456789"
	defaultAppIDLength  = 10
	appIDPrefix         = "ODOOAPP"
)

func GenerateDefaultAppID() string {
	result := make([]byte, defaultAppIDLength)
	max := big.NewInt(int64(len(charsetAlphanumeric)))

	for i := range result {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			// Fallback if crypto/rand fails.
			result[i] = charsetAlphanumeric[i%len(charsetAlphanumeric)]
			continue
		}
		result[i] = charsetAlphanumeric[n.Int64()]
	}

	return appIDPrefix + string(result)
}

func (cm *Manager) GetAppID() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.AppID
}
