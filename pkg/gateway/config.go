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
	"fmt"
	"time"
)

// FrontendConfig is the root configuration structure containing all frontends
type FrontendConfig struct {
	Frontends []FrontendDefinition `yaml:"frontends" json:"frontends"`
}

// FrontendDefinition defines a single frontend configuration
type FrontendDefinition struct {
	ID       string              `yaml:"id" json:"id"`             // Unique identifier for API operations
	Name     string              `yaml:"name" json:"name"`         // HAProxy frontend name
	Enabled  bool                `yaml:"enabled" json:"enabled"`   // Enable/disable this frontend
	Mode     string              `yaml:"mode" json:"mode"`         // "http" or "tcp"
	Bindings []BindingDefinition `yaml:"bindings" json:"bindings"` // Network binding configurations
	Routing  RoutingConfig       `yaml:"routing" json:"routing"`   // Routing behavior
	Options  FrontendOptions     `yaml:"options" json:"options"`   // Additional HAProxy options
}

// BindingDefinition defines how a frontend binds to the network
type BindingDefinition struct {
	Address  string `yaml:"address" json:"address"`   // IP address (e.g., "0.0.0.0", "::", "192.168.1.1")
	Port     int    `yaml:"port" json:"port"`         // Port number (1-65535)
	Protocol string `yaml:"protocol" json:"protocol"` // "http", "https", or "tcp"
	HTTP2    bool   `yaml:"http2" json:"http2"`       // Enable HTTP/2 support
	SSL      bool   `yaml:"ssl" json:"ssl"`           // Enable SSL/TLS
	ALPN     string `yaml:"alpn" json:"alpn"`         // ALPN protocol string (e.g., "h2,http/1.1")
	CertDir  string `yaml:"cert_dir" json:"cert_dir"` // Certificate directory path
	IPv6     bool   `yaml:"ipv6" json:"ipv6"`         // Enable IPv6 (v4v6 mode)
}

// RoutingConfig defines routing behavior for a frontend
type RoutingConfig struct {
	BypassRules    bool   `yaml:"bypass_rules" json:"bypass_rules"`       // Skip all ACL processing (Phase 4 feature)
	DefaultBackend string `yaml:"default_backend" json:"default_backend"` // Default backend name
}

// FrontendOptions holds additional HAProxy frontend options
type FrontendOptions struct {
	MaxConnections     int           `yaml:"max_connections" json:"max_connections"`           // Maximum concurrent connections
	TimeoutClient      time.Duration `yaml:"timeout_client" json:"timeout_client"`             // Client timeout
	HTTPConnectionMode string        `yaml:"http_connection_mode" json:"http_connection_mode"` // HTTP connection mode

	// Per-path overload protection (global-per-path rate limit driven by a map file).
	// When enabled, Bootstrap installs a stick-table, http-request rules and an empty
	// map file on this frontend at startup. Rules are then managed at runtime via
	// /api/frontends/{id}/overload.
	OverloadEnabled bool   `yaml:"overload_enabled" json:"overload_enabled"`
	OverloadPeriod  string `yaml:"overload_period" json:"overload_period"` // HAProxy time string, default "10s"
}

// Validate validates the entire frontend configuration
func (fc *FrontendConfig) Validate() error {
	if len(fc.Frontends) == 0 {
		return fmt.Errorf("at least one frontend must be defined")
	}

	// Track unique IDs and names
	ids := make(map[string]bool)
	names := make(map[string]bool)
	ports := make(map[string]string) // "address:port" -> frontend ID

	for i, fe := range fc.Frontends {
		// Validate individual frontend
		if err := fe.Validate(); err != nil {
			return fmt.Errorf("frontend[%d] (id=%s): %w", i, fe.ID, err)
		}

		// Check for duplicate IDs
		if ids[fe.ID] {
			return fmt.Errorf("duplicate frontend ID: %s", fe.ID)
		}
		ids[fe.ID] = true

		// Check for duplicate names
		if names[fe.Name] {
			return fmt.Errorf("duplicate frontend name: %s", fe.Name)
		}
		names[fe.Name] = true

		// Check for port conflicts
		for _, binding := range fe.Bindings {
			key := fmt.Sprintf("%s:%d", binding.Address, binding.Port)
			if existingID, exists := ports[key]; exists {
				return fmt.Errorf("port conflict: %s already used by frontend %s", key, existingID)
			}
			ports[key] = fe.ID
		}
	}

	return nil
}

// Validate validates a single frontend definition
func (fd *FrontendDefinition) Validate() error {
	if fd.ID == "" {
		return fmt.Errorf("id is required")
	}
	if fd.Name == "" {
		return fmt.Errorf("name is required")
	}
	if fd.Mode != "http" && fd.Mode != "tcp" {
		return fmt.Errorf("invalid mode: %s (must be 'http' or 'tcp')", fd.Mode)
	}
	if len(fd.Bindings) == 0 {
		return fmt.Errorf("at least one binding is required")
	}

	// Validate each binding
	for i, binding := range fd.Bindings {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("binding[%d]: %w", i, err)
		}
	}

	// Validate routing
	if err := fd.Routing.Validate(); err != nil {
		return fmt.Errorf("routing: %w", err)
	}

	return nil
}

// Validate validates a binding definition
func (bd *BindingDefinition) Validate() error {
	if bd.Address == "" {
		return fmt.Errorf("address is required")
	}
	if bd.Port <= 0 || bd.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be 1-65535)", bd.Port)
	}
	if bd.Protocol != "http" && bd.Protocol != "https" && bd.Protocol != "tcp" {
		return fmt.Errorf("invalid protocol: %s (must be 'http', 'https', or 'tcp')", bd.Protocol)
	}

	// SSL-specific validation
	if bd.SSL && bd.Protocol != "https" && bd.Protocol != "tcp" {
		return fmt.Errorf("ssl can only be enabled for 'https' or 'tcp' protocol")
	}

	// HTTP/2 specific validation
	if bd.HTTP2 && bd.Protocol == "tcp" {
		return fmt.Errorf("http2 cannot be enabled for 'tcp' protocol")
	}

	return nil
}

// Validate validates routing configuration
func (rc *RoutingConfig) Validate() error {
	if rc.DefaultBackend == "" {
		return fmt.Errorf("default_backend is required")
	}
	return nil
}

// SetDefaults sets default values for optional fields
func (fd *FrontendDefinition) SetDefaults() {
	// Set default mode if not specified
	if fd.Mode == "" {
		fd.Mode = "http"
	}

	// Set default enabled state
	if !fd.Enabled {
		fd.Enabled = true
	}

	// Set defaults for each binding
	for i := range fd.Bindings {
		fd.Bindings[i].SetDefaults()
	}

	// Set default options
	if fd.Options.HTTPConnectionMode == "" {
		fd.Options.HTTPConnectionMode = "http-keep-alive"
	}
}

// SetDefaults sets default values for binding
func (bd *BindingDefinition) SetDefaults() {
	// Set default address
	if bd.Address == "" {
		bd.Address = "0.0.0.0"
	}

	// Set default ALPN for HTTPS with HTTP/2
	if bd.SSL && bd.HTTP2 && bd.ALPN == "" {
		bd.ALPN = "h2,http/1.1"
	}

	// Set default protocol based on SSL flag
	if bd.Protocol == "" {
		if bd.SSL {
			bd.Protocol = "https"
		} else {
			bd.Protocol = "http"
		}
	}
}

// GetFrontendByID finds a frontend by its ID
func (fc *FrontendConfig) GetFrontendByID(id string) (*FrontendDefinition, error) {
	for i := range fc.Frontends {
		if fc.Frontends[i].ID == id {
			return &fc.Frontends[i], nil
		}
	}
	return nil, fmt.Errorf("frontend with ID %s not found", id)
}

// GetFrontendByName finds a frontend by its name
func (fc *FrontendConfig) GetFrontendByName(name string) (*FrontendDefinition, error) {
	for i := range fc.Frontends {
		if fc.Frontends[i].Name == name {
			return &fc.Frontends[i], nil
		}
	}
	return nil, fmt.Errorf("frontend with name %s not found", name)
}

// GetEnabledFrontends returns only enabled frontends
func (fc *FrontendConfig) GetEnabledFrontends() []FrontendDefinition {
	enabled := make([]FrontendDefinition, 0, len(fc.Frontends))
	for _, fe := range fc.Frontends {
		if fe.Enabled {
			enabled = append(enabled, fe)
		}
	}
	return enabled
}
