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
	dbURL  string
	token  string
	dbUUID string

	liveStatus      atomic.Pointer[string]
	lastContactTime atomic.Int64

	listenersMu sync.RWMutex
	listeners   []StatusListener

	workerMu     sync.Mutex
	workerCancel context.CancelFunc

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
		ctx:         ctx,
		cancel:      cancel,
	}

	if cfg.HasOdooCredentials() {
		odooCfg := cfg.GetOdooConfig()
		m.dbURL = odooCfg.DbURL
		m.token = odooCfg.Token
		m.dbUUID = odooCfg.DbUUID
		logger.Infof("[obox] Restored Odoo credentials from storage: db=%s dbuuid=%s", odooCfg.DbURL, odooCfg.DbUUID)
		m.startQueueHandler()
	}

	m.setLiveStatus("disconnected")
	return m
}

func (m *Module) startQueueHandler() {
	m.workerMu.Lock()
	defer m.workerMu.Unlock()

	if m.workerCancel != nil || m.ctx.Err() != nil {
		return
	}

	dbURL, token := m.GetCredentials()
	if dbURL == "" || token == "" {
		return
	}

	workerCtx, cancel := context.WithCancel(m.ctx)
	m.workerCancel = cancel
	go m.oboxQueueHandler(workerCtx)
}

func (m *Module) stopQueueHandler() {
	m.workerMu.Lock()
	defer m.workerMu.Unlock()

	if m.workerCancel != nil {
		m.workerCancel()
		m.workerCancel = nil
	}
}

func (m *Module) Stop() {
	m.stopQueueHandler()
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Module) SetCredentials(dbURL, token, dbUUID string) {
	m.credMu.Lock()
	m.dbURL = dbURL
	m.token = token
	m.dbUUID = dbUUID
	m.credMu.Unlock()
	m.startQueueHandler()
	m.notifyStatusChange()
}

func (m *Module) GetCredentials() (dbURL, token string) {
	m.credMu.RLock()
	defer m.credMu.RUnlock()
	if m.dbURL != "" {
		dbURL = m.dbURL
	}
	if m.token != "" {
		token = m.token
	}
	return dbURL, token
}

func (m *Module) ClearCredentials() {
	m.credMu.Lock()
	m.dbURL = ""
	m.token = ""
	m.dbUUID = ""
	m.credMu.Unlock()
}

func (m *Module) GetDbURL() string {
	m.credMu.RLock()
	defer m.credMu.RUnlock()
	if m.dbURL != "" {
		return m.dbURL
	}
	return ""
}

func (m *Module) GetWebsocketStatus() string {
	if ptr := m.liveStatus.Load(); ptr != nil && *ptr != "" {
		return *ptr
	}
	return "disconnected"
}

func (m *Module) Disconnect() {
	logger.Infof("[obox] Disconnect triggered")
	m.stopQueueHandler()
	if err := m.cfg.ClearOdooConfig(); err != nil {
		logger.Warnf("[obox] Failed to clear Odoo credentials from storage: %v", err)
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
