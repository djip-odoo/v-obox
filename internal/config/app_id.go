package config

import (
	"crypto/rand"
	"math/big"
)

const charsetAlphanumericUpper = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
const defaultAppIDLength = 10

// GenerateDefaultAppID generates a secure 10-character alphanumeric uppercase ID.
func GenerateDefaultAppID() string {
	b := make([]byte, defaultAppIDLength)
	maxIdx := big.NewInt(int64(len(charsetAlphanumericUpper)))
	for i := range defaultAppIDLength {
		n, err := rand.Int(rand.Reader, maxIdx)
		if err != nil {
			b[i] = charsetAlphanumericUpper[i%len(charsetAlphanumericUpper)]
		} else {
			b[i] = charsetAlphanumericUpper[n.Int64()]
		}
	}
	return string(b)
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

	cm.Data.AppID = GenerateDefaultAppID()
	if err := cm.saveLocked(); err != nil {
		return cm.Data.AppID, err
	}

	return cm.Data.AppID, nil
}
