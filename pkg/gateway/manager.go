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
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/instance"
	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

var logger = utils.GetLogger()

// Manager handles backend events and updates HAProxy configuration
type Manager struct {
	haproxyClient api.HAProxyClient
	provider      BackendProvider
	eventChan     chan BackendEvent
	stopChan      chan struct{}
	wg            sync.WaitGroup
	mu            sync.RWMutex
	backends      map[string]*Backend
	syncPeriod    time.Duration
}

// ManagerConfig holds configuration for the Manager
type ManagerConfig struct {
	HAProxyClient api.HAProxyClient
	Provider      BackendProvider
	SyncPeriod    time.Duration // How often to reconcile HAProxy config
	EventChanSize int           // Size of event channel buffer
}

// NewManager creates a new gateway manager
func NewManager(config ManagerConfig) *Manager {
	if config.SyncPeriod == 0 {
		config.SyncPeriod = 5 * time.Second
	}
	if config.EventChanSize == 0 {
		config.EventChanSize = 100
	}

	return &Manager{
		haproxyClient: config.HAProxyClient,
		provider:      config.Provider,
		eventChan:     make(chan BackendEvent, config.EventChanSize),
		stopChan:      make(chan struct{}),
		backends:      make(map[string]*Backend),
		syncPeriod:    config.SyncPeriod,
	}
}

// Start begins processing backend events and syncing with HAProxy
func (m *Manager) Start(ctx context.Context) error {
	logger.Info("Starting Gateway Manager")

	// Start the backend provider
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if err := m.provider.Start(ctx, m.eventChan); err != nil {
			logger.Errorf("Backend provider error: %v", err)
		}
	}()

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

	if err := m.provider.Stop(); err != nil {
		logger.Errorf("Error stopping provider: %v", err)
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
	m.mu.Lock()
	defer m.mu.Unlock()

	logger.Infof("Handling backend event: %s for backend %s", event.Type, event.Backend.Name)

	switch event.Type {
	case BackendEventAdd, BackendEventUpdate:
		backend := event.Backend
		m.backends[backend.Name] = &backend
		if err := m.syncBackendToHAProxy(&backend); err != nil {
			logger.Errorf("Error syncing backend %s: %v", backend.Name, err)
		}
	case BackendEventDelete:
		delete(m.backends, event.Backend.Name)
		m.haproxyClient.BackendDelete(event.Backend.Name)
	}
}

// syncBackendToHAProxy updates HAProxy configuration for a backend
func (m *Manager) syncBackendToHAProxy(backend *Backend) error {
	logger.Debugf("Syncing backend %s to HAProxy", backend.Name)

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

	_, created := m.haproxyClient.BackendCreateOrUpdate(haproxyBackend)
	if created {
		logger.Infof("Created backend: %s", backend.Name)
		instance.Reload("backend '%s' created", backend.Name)
	}

	// Delete all existing servers first (for clean state)
	if err := m.haproxyClient.BackendServerDeleteAll(backend.Name); err != nil {
		logger.Debugf("No servers to delete in backend %s", backend.Name)
	}

	// Add servers
	for _, srv := range backend.Servers {
		server := models.Server{
			Name:    srv.Name,
			Address: srv.IP,
			Port:    utils.PtrInt64(int64(srv.Port)),
		}

		if err := m.haproxyClient.BackendServerCreate(backend.Name, server); err != nil {
			logger.Errorf("Failed to create server %s in backend %s: %v", srv.Name, backend.Name, err)
		} else {
			logger.Debugf("Added server %s (%s:%d) to backend %s", srv.Name, srv.IP, srv.Port, backend.Name)
		}
	}

	// Commit transaction
	if err := m.haproxyClient.APICommitTransaction(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Final commit (processes backends)
	if err := m.haproxyClient.APIFinalCommitTransaction(); err != nil {
		return fmt.Errorf("failed to final commit transaction: %w", err)
	}

	logger.Infof("Successfully synced backend %s with %d servers", backend.Name, len(backend.Servers))
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
	m.mu.RLock()
	defer m.mu.RUnlock()

	logger.Debug("Running periodic reconciliation")

	// Get current backends from provider
	backends, err := m.provider.GetBackends()
	if err != nil {
		logger.Errorf("Failed to get backends from provider: %v", err)
		return
	}

	// Update local state and sync
	for _, backend := range backends {
		b := backend
		m.backends[backend.Name] = &b
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
