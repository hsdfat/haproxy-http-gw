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
)

// BackendServer represents a single backend server with IP and name
type BackendServer struct {
	Name string // Server name/identifier
	IP   string // IP address
	Port int    // Port number
}

// Backend represents a group of backend servers
type Backend struct {
	Name    string          // Backend name
	Servers []BackendServer // List of servers in this backend
}

// BackendEvent represents a change in backend configuration
type BackendEvent struct {
	Type    BackendEventType // Type of event (ADD, UPDATE, DELETE)
	Backend Backend          // Backend data
}

// BackendEventType represents the type of backend event
type BackendEventType string

const (
	BackendEventAdd    BackendEventType = "ADD"
	BackendEventUpdate BackendEventType = "UPDATE"
	BackendEventDelete BackendEventType = "DELETE"
)

// BackendProvider is the interface for providing backend information
// Implementations can fetch backends from various sources (database, REST API, etc.)
type BackendProvider interface {
	// Start begins watching for backend changes and sends events to the channel
	Start(ctx context.Context, eventChan chan<- BackendEvent) error

	// Stop stops the backend provider
	Stop() error

	// GetBackends returns the current list of all backends
	GetBackends() ([]Backend, error)

	// GetBackend returns a specific backend by name
	GetBackend(name string) (*Backend, error)
}
