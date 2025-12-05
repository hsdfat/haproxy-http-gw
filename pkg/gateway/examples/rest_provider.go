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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/haproxytech/kubernetes-ingress/pkg/gateway"
	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

// RESTBackendProvider fetches backends from a REST API
type RESTBackendProvider struct {
	mu           sync.RWMutex
	backends     map[string]gateway.Backend
	stopChan     chan struct{}
	pollInterval time.Duration
	apiURL       string
	httpClient   *http.Client
}

// RESTBackendResponse represents the JSON response from the REST API
type RESTBackendResponse struct {
	Backends []RESTBackend `json:"backends"`
}

// RESTBackend represents a backend in the REST API response
type RESTBackend struct {
	Name    string       `json:"name"`
	Servers []RESTServer `json:"servers"`
}

// RESTServer represents a server in the REST API response
type RESTServer struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// NewRESTBackendProvider creates a provider that fetches backends from a REST API
func NewRESTBackendProvider(apiURL string, pollInterval time.Duration) *RESTBackendProvider {
	if pollInterval == 0 {
		pollInterval = 10 * time.Second
	}

	return &RESTBackendProvider{
		backends:     make(map[string]gateway.Backend),
		stopChan:     make(chan struct{}),
		pollInterval: pollInterval,
		apiURL:       apiURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Start begins polling the REST API for backend changes
func (p *RESTBackendProvider) Start(ctx context.Context, eventChan chan<- gateway.BackendEvent) error {
	logger := utils.GetLogger()
	logger.Infof("Starting RESTBackendProvider, polling: %s", p.apiURL)

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	// Initial fetch
	if err := p.fetchAndUpdate(eventChan); err != nil {
		logger.Errorf("Initial fetch failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.stopChan:
			return nil
		case <-ticker.C:
			if err := p.fetchAndUpdate(eventChan); err != nil {
				logger.Errorf("Fetch failed: %v", err)
			}
		}
	}
}

// fetchAndUpdate fetches backends from REST API and sends events
func (p *RESTBackendProvider) fetchAndUpdate(eventChan chan<- gateway.BackendEvent) error {
	logger := utils.GetLogger()

	// Fetch from REST API
	resp, err := p.httpClient.Get(p.apiURL)
	if err != nil {
		return fmt.Errorf("failed to fetch backends: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Parse JSON response
	var restResponse RESTBackendResponse
	if err := json.Unmarshal(body, &restResponse); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Convert to internal format
	newBackends := make(map[string]gateway.Backend)
	for _, restBackend := range restResponse.Backends {
		servers := make([]gateway.BackendServer, len(restBackend.Servers))
		for i, srv := range restBackend.Servers {
			servers[i] = gateway.BackendServer{
				Name: srv.Name,
				IP:   srv.IP,
				Port: srv.Port,
			}
		}

		newBackends[restBackend.Name] = gateway.Backend{
			Name:    restBackend.Name,
			Servers: servers,
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Find added/updated backends
	for name, newBackend := range newBackends {
		oldBackend, exists := p.backends[name]

		if !exists {
			// New backend
			eventChan <- gateway.BackendEvent{
				Type:    gateway.BackendEventAdd,
				Backend: newBackend,
			}
			logger.Infof("New backend from REST API: %s", name)
		} else if !backendsEqual(oldBackend, newBackend) {
			// Updated backend
			eventChan <- gateway.BackendEvent{
				Type:    gateway.BackendEventUpdate,
				Backend: newBackend,
			}
			logger.Infof("Backend updated from REST API: %s", name)
		}
	}

	// Find deleted backends
	for name, oldBackend := range p.backends {
		if _, exists := newBackends[name]; !exists {
			eventChan <- gateway.BackendEvent{
				Type:    gateway.BackendEventDelete,
				Backend: oldBackend,
			}
			logger.Infof("Backend deleted from REST API: %s", name)
		}
	}

	// Update stored backends
	p.backends = newBackends
	logger.Debugf("Fetched %d backends from REST API", len(newBackends))
	return nil
}

// Stop stops the provider
func (p *RESTBackendProvider) Stop() error {
	logger := utils.GetLogger()
	logger.Info("Stopping RESTBackendProvider")
	close(p.stopChan)
	return nil
}

// GetBackends returns all backends
func (p *RESTBackendProvider) GetBackends() ([]gateway.Backend, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	backends := make([]gateway.Backend, 0, len(p.backends))
	for _, backend := range p.backends {
		backends = append(backends, backend)
	}
	return backends, nil
}

// GetBackend returns a specific backend
func (p *RESTBackendProvider) GetBackend(name string) (*gateway.Backend, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	backend, ok := p.backends[name]
	if !ok {
		return nil, nil
	}
	return &backend, nil
}
