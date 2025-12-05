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

package examples

import (
	"context"
	"sync"
	"time"

	"github.com/haproxytech/kubernetes-ingress/pkg/gateway"
	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

var logger = utils.GetLogger()

// SimpleProvider is an example implementation of BackendProvider
// that stores backends in memory and allows manual updates
type SimpleProvider struct {
	mu       sync.RWMutex
	backends map[string]gateway.Backend
	stopChan chan struct{}
}

// NewSimpleProvider creates a new simple backend provider
func NewSimpleProvider() *SimpleProvider {
	return &SimpleProvider{
		backends: make(map[string]gateway.Backend),
		stopChan: make(chan struct{}),
	}
}

// Start begins watching for backend changes
func (p *SimpleProvider) Start(ctx context.Context, eventChan chan<- gateway.BackendEvent) error {
	logger.Info("Starting SimpleProvider")

	// Send initial backends
	p.mu.RLock()
	for _, backend := range p.backends {
		select {
		case eventChan <- gateway.BackendEvent{
			Type:    gateway.BackendEventAdd,
			Backend: backend,
		}:
		case <-ctx.Done():
			p.mu.RUnlock()
			return ctx.Err()
		case <-p.stopChan:
			p.mu.RUnlock()
			return nil
		}
	}
	p.mu.RUnlock()

	// Keep the provider running (in real implementation, this would watch for changes)
	<-p.stopChan
	return nil
}

// Stop stops the provider
func (p *SimpleProvider) Stop() error {
	logger.Info("Stopping SimpleProvider")
	close(p.stopChan)
	return nil
}

// GetBackends returns all backends
func (p *SimpleProvider) GetBackends() ([]gateway.Backend, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	backends := make([]gateway.Backend, 0, len(p.backends))
	for _, backend := range p.backends {
		backends = append(backends, backend)
	}
	return backends, nil
}

// GetBackend returns a specific backend
func (p *SimpleProvider) GetBackend(name string) (*gateway.Backend, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	backend, ok := p.backends[name]
	if !ok {
		return nil, nil
	}
	return &backend, nil
}

// AddBackend manually adds a backend (for testing/demo)
func (p *SimpleProvider) AddBackend(backend gateway.Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.backends[backend.Name] = backend
	logger.Infof("Added backend: %s", backend.Name)
}

// UpdateBackend manually updates a backend (for testing/demo)
func (p *SimpleProvider) UpdateBackend(backend gateway.Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.backends[backend.Name] = backend
	logger.Infof("Updated backend: %s", backend.Name)
}

// DeleteBackend manually deletes a backend (for testing/demo)
func (p *SimpleProvider) DeleteBackend(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.backends, name)
	logger.Infof("Deleted backend: %s", name)
}

// PollingProvider polls an external source for backend updates
type PollingProvider struct {
	mu           sync.RWMutex
	backends     map[string]gateway.Backend
	stopChan     chan struct{}
	pollInterval time.Duration
	fetchFunc    func() ([]gateway.Backend, error)
}

// NewPollingProvider creates a provider that polls a function for backends
func NewPollingProvider(pollInterval time.Duration, fetchFunc func() ([]gateway.Backend, error)) *PollingProvider {
	if pollInterval == 0 {
		pollInterval = 10 * time.Second
	}

	return &PollingProvider{
		backends:     make(map[string]gateway.Backend),
		stopChan:     make(chan struct{}),
		pollInterval: pollInterval,
		fetchFunc:    fetchFunc,
	}
}

// Start begins polling for backend changes
func (p *PollingProvider) Start(ctx context.Context, eventChan chan<- gateway.BackendEvent) error {
	logger.Infof("Starting PollingProvider with interval: %s", p.pollInterval)

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	// Initial fetch
	if err := p.pollAndUpdate(eventChan); err != nil {
		logger.Errorf("Initial poll failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.stopChan:
			return nil
		case <-ticker.C:
			if err := p.pollAndUpdate(eventChan); err != nil {
				logger.Errorf("Poll failed: %v", err)
			}
		}
	}
}

// pollAndUpdate fetches backends and sends update events
func (p *PollingProvider) pollAndUpdate(eventChan chan<- gateway.BackendEvent) error {
	newBackends, err := p.fetchFunc()
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Convert to map for comparison
	newBackendsMap := make(map[string]gateway.Backend)
	for _, backend := range newBackends {
		newBackendsMap[backend.Name] = backend
	}

	// Find added/updated backends
	for name, newBackend := range newBackendsMap {
		oldBackend, exists := p.backends[name]

		if !exists {
			// New backend
			eventChan <- gateway.BackendEvent{
				Type:    gateway.BackendEventAdd,
				Backend: newBackend,
			}
			logger.Infof("Detected new backend: %s", name)
		} else if !backendsEqual(oldBackend, newBackend) {
			// Updated backend
			eventChan <- gateway.BackendEvent{
				Type:    gateway.BackendEventUpdate,
				Backend: newBackend,
			}
			logger.Infof("Detected backend update: %s", name)
		}
	}

	// Find deleted backends
	for name, oldBackend := range p.backends {
		if _, exists := newBackendsMap[name]; !exists {
			eventChan <- gateway.BackendEvent{
				Type:    gateway.BackendEventDelete,
				Backend: oldBackend,
			}
			logger.Infof("Detected backend deletion: %s", name)
		}
	}

	// Update stored backends
	p.backends = newBackendsMap
	return nil
}

// Stop stops the polling provider
func (p *PollingProvider) Stop() error {
	logger.Info("Stopping PollingProvider")
	close(p.stopChan)
	return nil
}

// GetBackends returns all backends
func (p *PollingProvider) GetBackends() ([]gateway.Backend, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	backends := make([]gateway.Backend, 0, len(p.backends))
	for _, backend := range p.backends {
		backends = append(backends, backend)
	}
	return backends, nil
}

// GetBackend returns a specific backend
func (p *PollingProvider) GetBackend(name string) (*gateway.Backend, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	backend, ok := p.backends[name]
	if !ok {
		return nil, nil
	}
	return &backend, nil
}

// backendsEqual compares two backends for equality
func backendsEqual(a, b gateway.Backend) bool {
	if a.Name != b.Name || len(a.Servers) != len(b.Servers) {
		return false
	}

	// Create maps for easier comparison
	aServers := make(map[string]gateway.BackendServer)
	for _, srv := range a.Servers {
		aServers[srv.Name] = srv
	}

	for _, bSrv := range b.Servers {
		aSrv, ok := aServers[bSrv.Name]
		if !ok || aSrv.IP != bSrv.IP || aSrv.Port != bSrv.Port {
			return false
		}
	}

	return true
}
