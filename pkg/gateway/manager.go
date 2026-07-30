// Copyright 2019 HAProxy Technologies LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/haproxytech/client-native/v6/models"
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

var logger = utils.GetLogger()

// Manager handles backend events and updates HAProxy configuration
type Manager struct {
	haproxyClient   api.HAProxyClient
	provider        BackendProvider
	eventChan       chan BackendEvent
	stopChan        chan struct{}
	wg              sync.WaitGroup
	mu              sync.RWMutex
	txMu            sync.Mutex // Mutex to serialize HAProxy transactions
	backends        map[string]*Backend
	runtimeBackends map[string]*RuntimeBackend // Track runtime state for dynamic updates
	syncPeriod      time.Duration
	serverSlotSize  int // Number of pre-allocated server slots (default: 42)
}

// ManagerConfig holds configuration for the Manager
type ManagerConfig struct {
	HAProxyClient  api.HAProxyClient
	Provider       BackendProvider
	SyncPeriod     time.Duration // How often to reconcile HAProxy config
	EventChanSize  int           // Size of event channel buffer
	ServerSlotSize int           // Number of pre-allocated server slots per backend (default: 42)
}

// NewManager creates a new gateway manager
func NewManager(config ManagerConfig) *Manager {
	if config.SyncPeriod == 0 {
		config.SyncPeriod = 5 * time.Second
	}
	if config.EventChanSize == 0 {
		config.EventChanSize = 100
	}
	if config.ServerSlotSize == 0 {
		config.ServerSlotSize = 42
	}

	return &Manager{
		haproxyClient:   config.HAProxyClient,
		provider:        config.Provider,
		eventChan:       make(chan BackendEvent, config.EventChanSize),
		stopChan:        make(chan struct{}),
		backends:        make(map[string]*Backend),
		runtimeBackends: make(map[string]*RuntimeBackend),
		syncPeriod:      config.SyncPeriod,
		serverSlotSize:  config.ServerSlotSize,
	}
}

// Start begins processing backend events and syncing with HAProxy
func (m *Manager) Start(ctx context.Context) error {
	logger.Info("Starting Gateway Manager")

	// Start the backend provider if one is configured
	if m.provider != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if err := m.provider.Start(ctx, m.eventChan); err != nil {
				logger.Errorf("Backend provider error: %v", err)
			}
		}()
	}

	// Start the event processor
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.processEvents(ctx)
	}()

	// Start the periodic sync
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.periodicSync(ctx)
	}()

	return nil
}

// Stop stops the gateway manager
func (m *Manager) Stop() error {
	logger.Info("Stopping Gateway Manager")
	close(m.stopChan)

	// The provider is optional (Start already treats nil as "no provider");
	// without this guard every graceful shutdown of a provider-less Manager
	// panicked here.
	if m.provider != nil {
		if err := m.provider.Stop(); err != nil {
			logger.Errorf("Error stopping provider: %v", err)
		}
	}

	m.wg.Wait()
	close(m.eventChan)

	return nil
}

// processEvents handles incoming backend events
func (m *Manager) processEvents(ctx context.Context) {
	logger.Info("Starting event processor")

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case event, ok := <-m.eventChan:
			if !ok {
				return
			}
			m.handleBackendEvent(event)
		}
	}
}

// handleBackendEvent processes a single backend event
func (m *Manager) handleBackendEvent(event BackendEvent) {
	logger.Infof("Handling backend event: %s for backend %s", event.Type, event.Backend.Name)

	// Map updates hold m.mu on their own; the HAProxy sync/delete runs after it
	// is released. syncBackendToHAProxy takes m.mu itself (calling it with the
	// write lock held self-deadlocks — RegisterBackend already uses this shape),
	// and clientBackendDelete takes the client's transaction lock, which
	// configUpdate holds while acquiring m.mu — so taking it under m.mu would
	// invert that order and deadlock.
	switch event.Type {
	case BackendEventAdd, BackendEventUpdate:
		backend := event.Backend
		m.mu.Lock()
		m.backends[backend.Name] = &backend
		m.mu.Unlock()
		if err := m.syncBackendToHAProxy(&backend); err != nil {
			logger.Errorf("Error syncing backend %s: %v", backend.Name, err)
		}
	case BackendEventDelete:
		m.mu.Lock()
		delete(m.backends, event.Backend.Name)
		m.mu.Unlock()
		m.clientBackendDelete(event.Backend.Name)
	}
}

// clientBackendDelete removes a backend from the shared client's staging map.
// BackendDelete mutates state that final commits iterate under the client's
// transaction lock, so the deletion has to hold that lock too. The lock is
// taken here rather than inside BackendDelete itself because handler code
// calls BackendDelete mid-transaction, when the lock is already held. Callers
// must not hold m.mu (see handleBackendEvent).
func (m *Manager) clientBackendDelete(name string) {
	if locker, ok := m.haproxyClient.(api.TransactionLocker); ok {
		locker.TxLock()
		defer locker.TxUnlock()
	}
	m.haproxyClient.BackendDelete(name)
}

// syncBackendToHAProxy updates HAProxy configuration for a backend
// It tries runtime socket update first, falling back to config reload if necessary
func (m *Manager) syncBackendToHAProxy(backend *Backend) error {
	logger.Debugf("Syncing backend %s to HAProxy", backend.Name)

	// The whole runtime-update attempt runs under m.mu: tryRuntimeUpdate reads
	// and mutates the shared per-slot state (Address/Port/Modified), which used
	// to happen with no lock at all after the entry was fetched under RLock. The
	// lock is released before configUpdate — that path acquires m.mu while
	// holding the client's transaction lock, so entering it with m.mu held
	// would invert the order.
	m.mu.Lock()
	existing, exists := m.runtimeBackends[backend.Name]
	if exists {
		// Try runtime update first (no reload)
		logger.Tracef("[RUNTIME] Attempting runtime update for backend %s", backend.Name)
		if err := m.tryRuntimeUpdate(existing, backend); err == nil {
			logger.Infof("[RUNTIME] Successfully updated backend %s via runtime socket (no reload)", backend.Name)

			// Update in-memory state
			existing.Servers = backend.Servers
			m.mu.Unlock()
			return nil
		} else {
			// Runtime failed, fall through to config update
			logger.Warningf("[RUNTIME] Runtime update failed for %s, falling back to config reload: %v", backend.Name, err)
		}
	}
	m.mu.Unlock()

	// Config-based update (with reload)
	return m.configUpdate(backend)
}

// tryRuntimeUpdate attempts to update backend servers via HAProxy runtime socket
func (m *Manager) tryRuntimeUpdate(existing *RuntimeBackend, newBackend *Backend) error {
	logger.Tracef("[RUNTIME] [BACKEND] [SERVER] updating backend %s for haproxy servers update (address and state) through socket", newBackend.Name)

	// Check if we have enough server slots
	if len(newBackend.Servers) > len(existing.HAProxySrvs) {
		return fmt.Errorf("not enough server slots (%d needed, %d available)", len(newBackend.Servers), len(existing.HAProxySrvs))
	}

	// Build runtime server data
	runtimeData := []api.RuntimeServerData{}

	// Update active servers
	for i, newSrv := range newBackend.Servers {
		slot := existing.HAProxySrvs[i]

		// Check if server changed
		if slot.Address != newSrv.IP || slot.Port != newSrv.Port {
			logger.Tracef("[RUNTIME] [BACKEND] [SERVER] [SOCKET] backend %s: server '%s': addr '%s:%d' changed status to ready",
				newBackend.Name, slot.Name, newSrv.IP, newSrv.Port)
			runtimeData = append(runtimeData, api.RuntimeServerData{
				BackendName: newBackend.Name,
				ServerName:  slot.Name,
				IP:          newSrv.IP,
				Port:        newSrv.Port,
				State:       "ready",
			})

			// Update in-memory state
			slot.Address = newSrv.IP
			slot.Port = newSrv.Port
			slot.Modified = true
		}
	}

	// Disable unused slots
	for i := len(newBackend.Servers); i < len(existing.HAProxySrvs); i++ {
		slot := existing.HAProxySrvs[i]
		if slot.Address != "" {
			logger.Tracef("[RUNTIME] [BACKEND] [SERVER] [SOCKET] backend %s: server '%s' changed status to maint",
				newBackend.Name, slot.Name)
			runtimeData = append(runtimeData, api.RuntimeServerData{
				BackendName: newBackend.Name,
				ServerName:  slot.Name,
				IP:          "127.0.0.1",
				Port:        1,
				State:       "maint",
			})

			// Update in-memory state
			slot.Address = ""
			slot.Port = 0
			slot.Modified = true
		}
	}

	// Execute runtime update
	if len(runtimeData) > 0 {
		err := m.haproxyClient.SetServerAddrAndState(runtimeData)
		if err != nil {
			existing.DynUpdateFailed = true
			return fmt.Errorf("runtime socket update failed: %w", err)
		}

		// Reset modified flags
		for _, slot := range existing.HAProxySrvs {
			slot.Modified = false
		}
	}

	return nil
}

// configUpdate performs a full config-based backend update with server slot pre-allocation
func (m *Manager) configUpdate(backend *Backend) error {
	logger.Debugf("[CONFIG] Updating backend %s via configuration (reload required)", backend.Name)

	// Serialize transactions to avoid race conditions
	m.txMu.Lock()
	defer m.txMu.Unlock()

	// Start transaction
	if err := m.haproxyClient.APIStartTransaction(); err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer m.haproxyClient.APIDisposeTransaction()

	// Create or update backend
	haproxyBackend := models.Backend{
		BackendBase: models.BackendBase{
			Name: backend.Name,
			Mode: "http",
			Balance: &models.Balance{
				Algorithm: utils.PtrString("roundrobin"),
			},
		},
	}

	// Use BackendCreatePermanently to ensure backends are not deleted by other transactions
	m.haproxyClient.BackendCreatePermanently(haproxyBackend)
	logger.Infof("[CONFIG] Created/updated backend: %s", backend.Name)

	// Delete all existing servers first (for clean state)
	if err := m.haproxyClient.BackendServerDeleteAll(backend.Name); err != nil {
		logger.Debugf("[CONFIG] No servers to delete in backend %s", backend.Name)
	}

	// Calculate total slots needed (round up to nearest multiple of serverSlotSize)
	totalSlots := len(backend.Servers)
	if totalSlots%m.serverSlotSize != 0 || totalSlots == 0 {
		totalSlots = ((totalSlots / m.serverSlotSize) + 1) * m.serverSlotSize
	}

	logger.Debugf("[CONFIG] [BACKEND] [SERVER] Pre-allocating %d server slots for backend %s (%d active, %d disabled)",
		totalSlots, backend.Name, len(backend.Servers), totalSlots-len(backend.Servers))

	// Track runtime backend state
	haproxySrvs := make([]*HAProxySrv, totalSlots)

	// Add active servers
	for i, srv := range backend.Servers {
		server := models.Server{
			Name:    fmt.Sprintf("SRV_%d", i+1),
			Address: srv.IP,
			Port:    utils.PtrInt64(int64(srv.Port)),
			ServerParams: models.ServerParams{
				Maintenance: "disabled",
			},
		}

		if err := m.haproxyClient.BackendServerCreate(backend.Name, server); err != nil {
			logger.Errorf("[CONFIG] Failed to create server %s in backend %s: %v", server.Name, backend.Name, err)
		} else {
			logger.Tracef("[CONFIG] [BACKEND] [SERVER] Added server %s (%s:%d) to backend %s",
				server.Name, srv.IP, srv.Port, backend.Name)
		}

		haproxySrvs[i] = &HAProxySrv{
			Name:     server.Name,
			Address:  srv.IP,
			Port:     srv.Port,
			Modified: false,
		}
	}

	// Add disabled (pre-allocated) slots
	for i := len(backend.Servers); i < totalSlots; i++ {
		server := models.Server{
			Name:    fmt.Sprintf("SRV_%d", i+1),
			Address: "127.0.0.1",
			Port:    utils.PtrInt64(1),
			ServerParams: models.ServerParams{
				Maintenance: "enabled",
			},
		}

		if err := m.haproxyClient.BackendServerCreate(backend.Name, server); err != nil {
			logger.Errorf("[CONFIG] Failed to create disabled server slot %s in backend %s: %v",
				server.Name, backend.Name, err)
		} else {
			logger.Tracef("[CONFIG] [BACKEND] [SERVER] Added disabled server slot %s to backend %s",
				server.Name, backend.Name)
		}

		haproxySrvs[i] = &HAProxySrv{
			Name:     server.Name,
			Address:  "",
			Port:     0,
			Modified: false,
		}
	}

	// Final commit (processes backends and commits the transaction)
	if err := m.haproxyClient.APIFinalCommitTransaction(); err != nil {
		return fmt.Errorf("failed to final commit transaction: %w", err)
	}

	// Store runtime backend state
	m.mu.Lock()
	m.runtimeBackends[backend.Name] = &RuntimeBackend{
		Name:            backend.Name,
		Servers:         backend.Servers,
		HAProxySrvs:     haproxySrvs,
		DynUpdateFailed: false,
	}
	m.mu.Unlock()

	logger.Infof("[CONFIG] Successfully synced backend %s with %d servers (%d total slots)",
		backend.Name, len(backend.Servers), totalSlots)
	return nil
}

// periodicSync periodically reconciles all backends
func (m *Manager) periodicSync(ctx context.Context) {
	ticker := time.NewTicker(m.syncPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.reconcile()
		}
	}
}

// reconcile ensures HAProxy state matches the desired state
func (m *Manager) reconcile() {
	logger.Debug("Running periodic reconciliation")

	// Skip reconciliation if no provider is configured. The provider field is
	// immutable after construction, so no lock is needed to read it — and no
	// lock may be held around the loop below: syncBackendToHAProxy takes m.mu
	// itself (holding it here self-deadlocked), and configUpdate acquires m.mu
	// while holding the client's transaction lock, so entering it with m.mu
	// held inverts that order.
	if m.provider == nil {
		logger.Debug("No provider configured, skipping reconciliation")
		return
	}

	// Get current backends from provider
	backends, err := m.provider.GetBackends()
	if err != nil {
		logger.Errorf("Failed to get backends from provider: %v", err)
		return
	}

	// Update local state and sync
	for _, backend := range backends {
		b := backend
		m.mu.Lock()
		m.backends[backend.Name] = &b
		m.mu.Unlock()
		if err := m.syncBackendToHAProxy(&b); err != nil {
			logger.Errorf("Reconciliation error for backend %s: %v", backend.Name, err)
		}
	}

	logger.Debugf("Reconciliation complete, managing %d backends", len(backends))
}

// GetBackends returns the current list of managed backends
func (m *Manager) GetBackends() map[string]*Backend {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*Backend, len(m.backends))
	for k, v := range m.backends {
		result[k] = v
	}
	return result
}

// RegisterBackend registers a new backend and syncs it to HAProxy
func (m *Manager) RegisterBackend(backend Backend) error {
	m.mu.Lock()
	m.backends[backend.Name] = &backend
	m.mu.Unlock()

	// Sync to HAProxy immediately
	if err := m.syncBackendToHAProxy(&backend); err != nil {
		logger.Errorf("Error syncing registered backend %s: %v", backend.Name, err)
		return err
	}

	logger.Infof("Backend %s registered successfully with %d servers", backend.Name, len(backend.Servers))
	return nil
}

// UnregisterBackend removes a backend and deletes it from HAProxy
func (m *Manager) UnregisterBackend(name string) error {
	m.mu.Lock()
	if _, exists := m.backends[name]; !exists {
		m.mu.Unlock()
		return fmt.Errorf("backend %s not found", name)
	}
	delete(m.backends, name)
	m.mu.Unlock()

	// Delete from HAProxy. Outside m.mu — see clientBackendDelete.
	m.clientBackendDelete(name)

	logger.Infof("Backend %s unregistered successfully", name)
	return nil
}
