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

package client

import (
	"fmt"

	"github.com/haproxytech/kubernetes-ingress/pkg/gateway"
)

// Client provides a programmatic interface to the HAProxy HTTP Gateway
type Client struct {
	frontendManager *gateway.FrontendManager
}

// NewClient creates a new gateway client
func NewClient(fm *gateway.FrontendManager) *Client {
	return &Client{
		frontendManager: fm,
	}
}

// RegisterBackend registers a backend with a specific frontend
// This is a programmatic alternative to using the HTTP API
func (c *Client) RegisterBackend(frontendID string, backend gateway.Backend) error {
	return c.frontendManager.RegisterBackend(frontendID, backend)
}

// UnregisterBackend removes a backend from a specific frontend
// This is a programmatic alternative to using the HTTP API
func (c *Client) UnregisterBackend(frontendID string, backendName string) error {
	return c.frontendManager.UnregisterBackend(frontendID, backendName)
}

// GetBackends returns all backends for a specific frontend
func (c *Client) GetBackends(frontendID string) (map[string]*gateway.Backend, error) {
	return c.frontendManager.GetBackends(frontendID)
}

// AddRoute adds a routing rule to a specific frontend
func (c *Client) AddRoute(frontendID string, route gateway.Route) error {
	return c.frontendManager.AddRoute(frontendID, route)
}

// DeleteRoute removes a routing rule from a specific frontend
func (c *Client) DeleteRoute(frontendID string, routeID string) error {
	return c.frontendManager.DeleteRoute(frontendID, routeID)
}

// GetRoutes returns all routes for a specific frontend
func (c *Client) GetRoutes(frontendID string) (map[string]gateway.Route, error) {
	return c.frontendManager.GetRoutes(frontendID)
}

// ListFrontends returns all configured frontends
func (c *Client) ListFrontends() map[string]*gateway.ManagedFrontend {
	return c.frontendManager.ListFrontends()
}

// GetFrontend returns a specific frontend by ID
func (c *Client) GetFrontend(frontendID string) (*gateway.ManagedFrontend, error) {
	return c.frontendManager.GetFrontend(frontendID)
}

// AddServer adds a single server to an existing backend
// This is a convenience method that fetches the backend, adds the server, and re-registers
func (c *Client) AddServer(frontendID, backendName string, server gateway.BackendServer) error {
	backends, err := c.GetBackends(frontendID)
	if err != nil {
		return err
	}

	backend, exists := backends[backendName]
	if !exists {
		return fmt.Errorf("backend %s not found", backendName)
	}

	backend.Servers = append(backend.Servers, server)
	return c.RegisterBackend(frontendID, *backend)
}

// RemoveServer removes a server from an existing backend by server name
func (c *Client) RemoveServer(frontendID, backendName, serverName string) error {
	backends, err := c.GetBackends(frontendID)
	if err != nil {
		return err
	}

	backend, exists := backends[backendName]
	if !exists {
		return fmt.Errorf("backend %s not found", backendName)
	}

	newServers := []gateway.BackendServer{}
	found := false
	for _, srv := range backend.Servers {
		if srv.Name != serverName {
			newServers = append(newServers, srv)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("server %s not found in backend %s", serverName, backendName)
	}

	backend.Servers = newServers
	return c.RegisterBackend(frontendID, *backend)
}

// UpdateServerAddress updates the IP and/or port of a specific server
func (c *Client) UpdateServerAddress(frontendID, backendName, serverName, newIP string, newPort int) error {
	backends, err := c.GetBackends(frontendID)
	if err != nil {
		return err
	}

	backend, exists := backends[backendName]
	if !exists {
		return fmt.Errorf("backend %s not found", backendName)
	}

	for i := range backend.Servers {
		if backend.Servers[i].Name == serverName {
			backend.Servers[i].IP = newIP
			backend.Servers[i].Port = newPort
			return c.RegisterBackend(frontendID, *backend)
		}
	}

	return fmt.Errorf("server %s not found in backend %s", serverName, backendName)
}
