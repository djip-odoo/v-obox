package obox

import (
	"context"
	"sync"
	"sync/atomic"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"
	"epos-proxy/internal/printer"
)

type StatusListener func()

type Module struct {
	appID       string
	cfg         *config.Manager
	mgr         *printer.Manager
	localAddrFn func() string

	credMu sync.RWMutex
	dbURL  *string
	token  *string
	dbUUID *string

	liveStatus      atomic.Pointer[string]
	lastContactTime atomic.Int64

	listenersMu sync.RWMutex
	listeners   []StatusListener

	triggerChan chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
}

func Manager(cfg *config.Manager, mgr *printer.Manager, localAddrFn func() string) *Module {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Module{
		cfg:         cfg,
		mgr:         mgr,
		localAddrFn: localAddrFn,
		appID:       cfg.GetAppID(),
		triggerChan: make(chan struct{}, 1),
		ctx:         ctx,
		cancel:      cancel,
	}

	if cfg.HasOdooCredentials() {
		odooCfg := cfg.GetOdooConfig()
		m.dbURL = &odooCfg.DbURL
		m.token = &odooCfg.Token
		m.dbUUID = &odooCfg.DbUUID
		logger.Infof("[obox] Restored Odoo credentials from storage: db=%s dbuuid=%s", odooCfg.DbURL, odooCfg.DbUUID)
	}

	m.setLiveStatus("disconnected")
	go m.oboxQueueHandler()
	go m.oboxWebsocketHandler()
	return m
}

func (m *Module) TriggerFetch() {
	if m.triggerChan == nil {
		return
	}
	select {
	case m.triggerChan <- struct{}{}:
	default:
	}
}

func (m *Module) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Module) SetCredentials(dbURL, token, dbUUID string) {
	m.credMu.Lock()
	m.dbURL = &dbURL
	m.token = &token
	m.dbUUID = &dbUUID
	m.credMu.Unlock()
	m.notifyStatusChange()
	m.TriggerFetch()
}

func (m *Module) GetCredentials() (dbURL, token string) {
	m.credMu.RLock()
	defer m.credMu.RUnlock()
	if m.dbURL != nil {
		dbURL = *m.dbURL
	}
	if m.token != nil {
		token = *m.token
	}
	return dbURL, token
}

func (m *Module) ClearCredentials() {
	m.credMu.Lock()
	m.dbURL = nil
	m.token = nil
	m.dbUUID = nil
	m.credMu.Unlock()
}

func (m *Module) IsConnected() bool {
	m.credMu.RLock()
	defer m.credMu.RUnlock()
	return m.dbURL != nil && m.token != nil && *m.dbURL != "" && *m.token != ""
}

func (m *Module) GetDbURL() string {
	m.credMu.RLock()
	defer m.credMu.RUnlock()
	if m.dbURL != nil && *m.dbURL != "" {
		return *m.dbURL
	}
	return ""
}

func (m *Module) GetWebsocketStatus() string {
	if !m.IsConnected() {
		return "disconnected"
	}
	if ptr := m.liveStatus.Load(); ptr != nil && *ptr != "" {
		return *ptr
	}
	return "disconnected"
}

func (m *Module) Disconnect() {
	logger.Infof("[obox] Disconnect triggered")
	if m.cfg != nil {
		if err := m.cfg.ClearOdooConfig(); err != nil {
			logger.Warnf("[obox] Failed to clear Odoo credentials from storage: %v", err)
		}
	}
	m.ClearCredentials()
	m.setLiveStatus("disconnected")
	m.notifyStatusChange()
}

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

	for _, listener := range listeners {
		listener()
	}
}

func (m *Module) setLiveStatus(st string) {
	prev := m.liveStatus.Load()
	m.liveStatus.Store(&st)
	if prev == nil || *prev != st {
		m.notifyStatusChange()
	}
}
