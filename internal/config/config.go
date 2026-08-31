package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var ErrNoAvailablePort = errors.New("no available port in range")

const AppName = "EposProxy"

const (
	PortRangeStart = 4545
	PortRangeEnd   = 4555
)

type AppConfig struct {
	Port            int      `json:"port"`
	LANPrinters     []string `json:"lan_printers,omitempty"`
	WebViewURL      string   `json:"webview_url,omitempty"`
	WebViewPIN      string   `json:"webview_pin,omitempty"`
	WebViewEnabled  bool     `json:"webview_enabled"`
	NetworkPrinting bool     `json:"network_printing"`
}

func defaults() AppConfig {
	return AppConfig{
		Port:            0,
		NetworkPrinting: false,
		WebViewPIN:      "0000",
	}
}

type Manager struct {
	mu   sync.RWMutex
	path string
	Data AppConfig
}

func getBaseConfigDir() string {
	base, err := os.UserConfigDir()
	if err == nil && base != "" {
		return base
	}
	if filesDir := os.Getenv("FILES_DIR"); filesDir != "" {
		return filesDir
	}
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if _, err := os.Stat("/data/data/com.wails.app/files"); err == nil {
		return "/data/data/com.wails.app/files"
	}
	return os.TempDir()
}

func NewManager() (*Manager, error) {
	base := getBaseConfigDir()
	dir := filepath.Join(base, AppName)
	_ = os.MkdirAll(dir, 0755)

	return &Manager{
		path: filepath.Join(dir, "config.json"),
		Data: defaults(),
	}, nil
}

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

func (cm *Manager) Save() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.saveLocked()
}

func (cm *Manager) saveLocked() error {
	data, err := json.MarshalIndent(cm.Data, "", "  ")
	if err != nil {
		return fmt.Errorf("config marshal error: %w", err)
	}
	if err := os.WriteFile(cm.path, data, 0644); err != nil {
		return fmt.Errorf("config write error: %w", err)
	}
	return nil
}

func (cm *Manager) Path() string { return cm.path }

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

	return 0, fmt.Errorf("no available port found in range %d-%d: %w", start, end, ErrNoAvailablePort)
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

func (cm *Manager) SetNetworkPrintingEnabled(enabled bool) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.Data.NetworkPrinting = enabled
	return cm.saveLocked()
}

func (cm *Manager) IsNetworkPrintingEnabled() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.NetworkPrinting
}

func (cm *Manager) AddLanEposPrinter(ip string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, existing := range cm.Data.LANPrinters {
		if existing == ip {
			return nil // Already exists
		}
	}
	cm.Data.LANPrinters = append(cm.Data.LANPrinters, ip)
	return cm.saveLocked()
}

func (cm *Manager) RemoveLANPrinter(ip string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, existing := range cm.Data.LANPrinters {
		if existing == ip {
			cm.Data.LANPrinters = append(cm.Data.LANPrinters[:i], cm.Data.LANPrinters[i+1:]...)
			return cm.saveLocked()
		}
	}
	return nil // Not found, nothing to remove
}

// GetWebViewURL returns the configured kiosk URL.
func (cm *Manager) GetWebViewURL() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.WebViewURL
}

// GetWebViewEnabled returns whether kiosk mode is enabled.
func (cm *Manager) GetWebViewEnabled() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.WebViewEnabled
}

// HasWebViewPIN reports whether a PIN has been configured.
func (cm *Manager) HasWebViewPIN() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.WebViewPIN != ""
}

// SetWebViewURL validates and persists the kiosk URL.
func (cm *Manager) SetWebViewURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed != "" {
		parsed, err := url.ParseRequestURI(trimmed)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return errors.New("invalid URL: must be a valid HTTP or HTTPS address")
		}
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.Data.WebViewURL = trimmed
	return cm.saveLocked()
}

// SetWebViewPIN validates (exactly 4 digits) and persists the plaintext PIN.
func (cm *Manager) SetWebViewPIN(pin string) error {
	if len(pin) != 4 {
		return errors.New("PIN must be exactly 4 digits")
	}
	for _, ch := range pin {
		if ch < '0' || ch > '9' {
			return errors.New("PIN must contain digits only")
		}
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.Data.WebViewPIN = pin
	return cm.saveLocked()
}

// CheckWebViewPIN returns true when raw matches the stored PIN.
func (cm *Manager) CheckWebViewPIN(raw string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.Data.WebViewPIN != "" && cm.Data.WebViewPIN == raw
}

// SetWebViewEnabled persists the enabled flag.
func (cm *Manager) SetWebViewEnabled(v bool) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if v && cm.Data.WebViewURL == "" {
		return errors.New("cannot enable kiosk mode: URL is not configured")
	}
	cm.Data.WebViewEnabled = v
	return cm.saveLocked()
}

func (cm *Manager) GetLANPrinters() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.Data.LANPrinters == nil {
		return []string{}
	}
	// Return a copy to avoid races if caller modifies the slice
	result := make([]string, len(cm.Data.LANPrinters))
	copy(result, cm.Data.LANPrinters)
	return result
}
