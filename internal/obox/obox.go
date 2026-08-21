package obox

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"epos-proxy/internal/config"
	"epos-proxy/internal/logger"
)

type StatusListener func()

type Module struct {
	appID string
	port  int
	cfg   *config.Manager

	credMu sync.RWMutex
	dbURL  string
	token  string
	dbUUID string

	wsStatus atomic.Pointer[string]

	lanStatus       atomic.Pointer[string]
	lastContactTime atomic.Int64
	lanMu           sync.Mutex
	lanTimer        *time.Timer

	listenersMu sync.RWMutex
	listeners   []StatusListener

	workerMu     sync.Mutex
	workerCancel context.CancelFunc

	ctx    context.Context
	cancel context.CancelFunc
}

func NewModule(cfg *config.Manager, port int) *Module {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Module{
		cfg:    cfg,
		port:   port,
		appID:  cfg.GetAppID(),
		ctx:    ctx,
		cancel: cancel,
	}
	m.setLANStatus("disconnected")

	if cfg.HasOdooCredentials() {
		odooCfg := cfg.GetOdooConfig()
		m.dbURL = odooCfg.DbURL
		m.token = odooCfg.Token
		m.dbUUID = odooCfg.DbUUID
		logger.Infof("[obox] Restored Odoo credentials from storage: db=%s dbuuid=%s", odooCfg.DbURL, odooCfg.DbUUID)
		m.setLiveStatus("connecting")
		m.startQueueHandler()
	} else {
		m.setLiveStatus("disconnected")
	}

	return m
}

func (m *Module) LocalAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", m.port)
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
	m.lanMu.Lock()
	if m.lanTimer != nil {
		m.lanTimer.Stop()
		m.lanTimer = nil
	}
	m.lanMu.Unlock()
	m.setLANStatus("disconnected")
}

func (m *Module) SetCredentials(dbURL, token, dbUUID string) {
	m.credMu.Lock()
	m.dbURL = dbURL
	m.token = token
	m.dbUUID = dbUUID
	if err := m.cfg.SetOdooCredentials(dbURL, token, dbUUID); err != nil {
		logger.Warnf("[obox] Failed to save Odoo credentials to storage: %v", err)
	}
	m.credMu.Unlock()
	m.startQueueHandler()
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
	if err := m.cfg.ClearOdooConfig(); err != nil {
		logger.Warnf("[obox] Failed to clear Odoo credentials from storage: %v", err)
	}
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
	if ptr := m.wsStatus.Load(); ptr != nil && *ptr != "" {
		return *ptr
	}
	return "disconnected"
}

func (m *Module) GetLANStatus() string {
	if ptr := m.lanStatus.Load(); ptr != nil && *ptr != "" {
		return *ptr
	}
	return "disconnected"
}

func (m *Module) Disconnect() {
	logger.Infof("[obox] Disconnect triggered")
	m.stopQueueHandler()
	m.ClearCredentials()
	m.setLiveStatus("disconnected")
	m.lanMu.Lock()
	if m.lanTimer != nil {
		m.lanTimer.Stop()
		m.lanTimer = nil
	}
	m.lanMu.Unlock()
	m.setLANStatus("disconnected")
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
	prev := m.wsStatus.Load()
	m.wsStatus.Store(&st)
	if prev == nil || *prev != st {
		m.notifyStatusChange()
	}
}

func (m *Module) setLANStatus(st string) {
	prev := m.lanStatus.Load()
	m.lanStatus.Store(&st)
	if prev == nil || *prev != st {
		m.notifyStatusChange()
	}
}

func (m *Module) RecordLANContact() {
	m.setLANStatus("connected")

	m.lanMu.Lock()
	defer m.lanMu.Unlock()
	if m.lanTimer != nil {
		m.lanTimer.Stop()
	}
	m.lanTimer = time.AfterFunc(30*time.Second, func() {
		m.setLANStatus("disconnected")
	})
}
