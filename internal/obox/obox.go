package obox

import (
	"sync"
	"sync/atomic"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"

	"github.com/gofiber/fiber/v3"
)

// StatusListener represents a callback function for Odoo status changes.
type StatusListener func()

// Module manages Odoo/Obox connection state, background polling, and device coordination.
type Module struct {
	appID       string
	cfg         *config.Manager
	localAddrFn func() string

	credMu sync.RWMutex
	dbURL  string
	token  string
	dbUUID string

	liveStatus      atomic.Pointer[string]
	lastContactTime atomic.Int64

	listenersMu sync.RWMutex
	listeners   []StatusListener
}

// New creates and initializes a new Obox Module.
func New(cfg *config.Manager, localAddrFn func() string) *Module {
	m := &Module{
		cfg:         cfg,
		localAddrFn: localAddrFn,
	}

	if cfg != nil {
		m.appID = cfg.GetAppID()
		if cfg.HasOdooCredentials() {
			odooCfg := cfg.GetOdooConfig()
			m.dbURL = odooCfg.DbURL
			m.token = odooCfg.Token
			m.dbUUID = odooCfg.DbUUID
			logger.Infof("[obox] Restored Odoo credentials from storage: db=%s dbuuid=%s", odooCfg.DbURL, odooCfg.DbUUID)
		}
	}

	m.setLiveStatus("disconnected")
	go m.deviceBrain()
	return m
}

// RegisterRoutes registers all Obox HTTP endpoints onto the Fiber app.
func (m *Module) RegisterRoutes(app *fiber.App) {
	app.Get("/odoo/", m.handleDiscovery)
	app.Get("/odoo/health", m.handleHealth)
	app.Get("/odoo/restart", m.handleRestart)
	app.Get("/odoo/disconnect", m.handleDisconnect)
	app.Get("/odoo/discover_devices", m.handleDiscoverDevices)
	app.Get("/odoo/connect", m.handleConnect)
}

// SetCredentials sets the in-memory Odoo credentials.
func (m *Module) SetCredentials(dbURL, token, dbUUID string) {
	m.credMu.Lock()
	m.dbURL = dbURL
	m.token = token
	m.dbUUID = dbUUID
	m.credMu.Unlock()
}

// GetCredentials retrieves the in-memory Odoo credentials safely.
func (m *Module) GetCredentials() (dbURL, token, dbUUID string) {
	m.credMu.RLock()
	defer m.credMu.RUnlock()
	return m.dbURL, m.token, m.dbUUID
}

// ClearCredentials clears in-memory credentials.
func (m *Module) ClearCredentials() {
	m.credMu.Lock()
	m.dbURL = ""
	m.token = ""
	m.dbUUID = ""
	m.credMu.Unlock()
}

// IsConnected reports whether valid Odoo credentials are configured.
func (m *Module) IsConnected() bool {
	m.credMu.RLock()
	defer m.credMu.RUnlock()
	return m.dbURL != "" && m.token != ""
}

// GetDbURL returns the current Odoo DB URL.
func (m *Module) GetDbURL() string {
	m.credMu.RLock()
	url := m.dbURL
	m.credMu.RUnlock()
	if url != "" {
		return url
	}
	if m.cfg != nil {
		return m.cfg.GetOdooDbURL()
	}
	return ""
}

// GetWebsocketStatus returns the live Obox connection status.
func (m *Module) GetWebsocketStatus() string {
	if !m.IsConnected() {
		return "disconnected"
	}
	if ptr := m.liveStatus.Load(); ptr != nil && *ptr != "" {
		return *ptr
	}
	return "disconnected"
}

// Disconnect clears in-memory device credentials and removes stored config.
func (m *Module) Disconnect() {
	logger.Infof("[obox] Disconnect triggered")
	m.ClearCredentials()
	m.setLiveStatus("disconnected")
	if m.cfg != nil {
		if err := m.cfg.ClearOdooConfig(); err != nil {
			logger.Warnf("[obox] Failed to clear Odoo credentials from storage: %v", err)
		}
	}
}

// OnStatusChange registers a callback to be called whenever OdooStatus changes.
func (m *Module) OnStatusChange(listener StatusListener) {
	m.listenersMu.Lock()
	defer m.listenersMu.Unlock()
	m.listeners = append(m.listeners, listener)
}

func (m *Module) notifyStatusChange() {
	m.listenersMu.RLock()
	listeners := make([]StatusListener, len(m.listeners))
	copy(listeners, m.listeners)
	m.listenersMu.RUnlock()

	for _, l := range listeners {
		l()
	}
}

func (m *Module) setLiveStatus(st string) {
	prev := m.liveStatus.Load()
	changed := prev == nil || *prev != st
	m.liveStatus.Store(&st)
	if changed {
		m.notifyStatusChange()
	}
}
