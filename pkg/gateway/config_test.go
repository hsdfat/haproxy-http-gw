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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

func TestFrontendConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  FrontendConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: FrontendConfig{
				Frontends: []FrontendDefinition{
					{
						ID:      "frontend-1",
						Name:    "http-gateway-1",
						Enabled: true,
						Mode:    "http",
						Bindings: []BindingDefinition{
							{Address: "0.0.0.0", Port: 8080, Protocol: "http"},
						},
						Routing: RoutingConfig{
							DefaultBackend: "default-backend",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "no frontends",
			config: FrontendConfig{
				Frontends: []FrontendDefinition{},
			},
			wantErr: true,
			errMsg:  "at least one frontend must be defined",
		},
		{
			name: "duplicate frontend IDs",
			config: FrontendConfig{
				Frontends: []FrontendDefinition{
					{
						ID:   "frontend-1",
						Name: "gateway-1",
						Mode: "http",
						Bindings: []BindingDefinition{
							{Address: "0.0.0.0", Port: 8080, Protocol: "http"},
						},
						Routing: RoutingConfig{DefaultBackend: "backend-1"},
					},
					{
						ID:   "frontend-1",
						Name: "gateway-2",
						Mode: "http",
						Bindings: []BindingDefinition{
							{Address: "0.0.0.0", Port: 8081, Protocol: "http"},
						},
						Routing: RoutingConfig{DefaultBackend: "backend-2"},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate frontend ID",
		},
		{
			name: "duplicate frontend names",
			config: FrontendConfig{
				Frontends: []FrontendDefinition{
					{
						ID:   "frontend-1",
						Name: "gateway",
						Mode: "http",
						Bindings: []BindingDefinition{
							{Address: "0.0.0.0", Port: 8080, Protocol: "http"},
						},
						Routing: RoutingConfig{DefaultBackend: "backend-1"},
					},
					{
						ID:   "frontend-2",
						Name: "gateway",
						Mode: "http",
						Bindings: []BindingDefinition{
							{Address: "0.0.0.0", Port: 8081, Protocol: "http"},
						},
						Routing: RoutingConfig{DefaultBackend: "backend-2"},
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate frontend name",
		},
		{
			name: "port conflict",
			config: FrontendConfig{
				Frontends: []FrontendDefinition{
					{
						ID:   "frontend-1",
						Name: "gateway-1",
						Mode: "http",
						Bindings: []BindingDefinition{
							{Address: "0.0.0.0", Port: 8080, Protocol: "http"},
						},
						Routing: RoutingConfig{DefaultBackend: "backend-1"},
					},
					{
						ID:   "frontend-2",
						Name: "gateway-2",
						Mode: "http",
						Bindings: []BindingDefinition{
							{Address: "0.0.0.0", Port: 8080, Protocol: "http"},
						},
						Routing: RoutingConfig{DefaultBackend: "backend-2"},
					},
				},
			},
			wantErr: true,
			errMsg:  "port conflict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if len(err.Error()) == 0 || err.Error()[:len(tt.errMsg)] != tt.errMsg {
					t.Errorf("Validate() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestFrontendDefinition_Validate(t *testing.T) {
	tests := []struct {
		name       string
		definition FrontendDefinition
		wantErr    bool
		errMsg     string
	}{
		{
			name: "valid definition",
			definition: FrontendDefinition{
				ID:   "test",
				Name: "test-gateway",
				Mode: "http",
				Bindings: []BindingDefinition{
					{Address: "0.0.0.0", Port: 8080, Protocol: "http"},
				},
				Routing: RoutingConfig{
					DefaultBackend: "test-backend",
				},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			definition: FrontendDefinition{
				Name: "test-gateway",
				Mode: "http",
				Bindings: []BindingDefinition{
					{Address: "0.0.0.0", Port: 8080, Protocol: "http"},
				},
				Routing: RoutingConfig{DefaultBackend: "test-backend"},
			},
			wantErr: true,
			errMsg:  "id is required",
		},
		{
			name: "missing name",
			definition: FrontendDefinition{
				ID:   "test",
				Mode: "http",
				Bindings: []BindingDefinition{
					{Address: "0.0.0.0", Port: 8080, Protocol: "http"},
				},
				Routing: RoutingConfig{DefaultBackend: "test-backend"},
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "invalid mode",
			definition: FrontendDefinition{
				ID:   "test",
				Name: "test-gateway",
				Mode: "invalid",
				Bindings: []BindingDefinition{
					{Address: "0.0.0.0", Port: 8080, Protocol: "http"},
				},
				Routing: RoutingConfig{DefaultBackend: "test-backend"},
			},
			wantErr: true,
			errMsg:  "invalid mode",
		},
		{
			name: "no bindings",
			definition: FrontendDefinition{
				ID:       "test",
				Name:     "test-gateway",
				Mode:     "http",
				Bindings: []BindingDefinition{},
				Routing:  RoutingConfig{DefaultBackend: "test-backend"},
			},
			wantErr: true,
			errMsg:  "at least one binding is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.definition.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if len(err.Error()) == 0 || err.Error()[:len(tt.errMsg)] != tt.errMsg {
					t.Errorf("Validate() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestBindingDefinition_Validate(t *testing.T) {
	tests := []struct {
		name    string
		binding BindingDefinition
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid HTTP binding",
			binding: BindingDefinition{Address: "0.0.0.0", Port: 8080, Protocol: "http"},
			wantErr: false,
		},
		{
			name:    "valid HTTPS binding",
			binding: BindingDefinition{Address: "0.0.0.0", Port: 8443, Protocol: "https", SSL: true},
			wantErr: false,
		},
		{
			name:    "missing address",
			binding: BindingDefinition{Port: 8080, Protocol: "http"},
			wantErr: true,
			errMsg:  "address is required",
		},
		{
			name:    "invalid port - zero",
			binding: BindingDefinition{Address: "0.0.0.0", Port: 0, Protocol: "http"},
			wantErr: true,
			errMsg:  "invalid port",
		},
		{
			name:    "invalid port - negative",
			binding: BindingDefinition{Address: "0.0.0.0", Port: -1, Protocol: "http"},
			wantErr: true,
			errMsg:  "invalid port",
		},
		{
			name:    "invalid port - too high",
			binding: BindingDefinition{Address: "0.0.0.0", Port: 65536, Protocol: "http"},
			wantErr: true,
			errMsg:  "invalid port",
		},
		{
			name:    "invalid protocol",
			binding: BindingDefinition{Address: "0.0.0.0", Port: 8080, Protocol: "invalid"},
			wantErr: true,
			errMsg:  "invalid protocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.binding.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if len(err.Error()) == 0 || err.Error()[:len(tt.errMsg)] != tt.errMsg {
					t.Errorf("Validate() error = %v, want error containing %v", err, tt.errMsg)
				}
			}
		})
	}
}

func TestFrontendDefinition_SetDefaults(t *testing.T) {
	def := FrontendDefinition{
		ID:   "test",
		Name: "test-gateway",
		Bindings: []BindingDefinition{
			{Port: 8080},
		},
	}

	def.SetDefaults()

	if def.Mode != "http" {
		t.Errorf("Expected default mode to be 'http', got %s", def.Mode)
	}

	if !def.Enabled {
		t.Errorf("Expected default enabled to be true")
	}

	if def.Options.HTTPConnectionMode != "http-keep-alive" {
		t.Errorf("Expected default HTTPConnectionMode to be 'http-keep-alive', got %s", def.Options.HTTPConnectionMode)
	}

	if def.Bindings[0].Address != "0.0.0.0" {
		t.Errorf("Expected default address to be '0.0.0.0', got %s", def.Bindings[0].Address)
	}
}

func TestYAMLConfigProvider_LoadConfig(t *testing.T) {
	// Create a temporary YAML config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "frontends.yaml")

	yamlContent := `frontends:
  - id: "test-frontend"
    name: "test-gateway"
    enabled: true
    mode: "http"
    bindings:
      - address: "0.0.0.0"
        port: 8080
        protocol: "http"
        http2: true
    routing:
      bypass_rules: false
      default_backend: "test-backend"
    options:
      max_connections: 10000
      timeout_client: "30s"
`

	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	provider := NewYAMLConfigProvider(configFile)

	config, err := provider.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if len(config.Frontends) != 1 {
		t.Errorf("Expected 1 frontend, got %d", len(config.Frontends))
	}

	fe := config.Frontends[0]
	if fe.ID != "test-frontend" {
		t.Errorf("Expected ID 'test-frontend', got %s", fe.ID)
	}
	if fe.Name != "test-gateway" {
		t.Errorf("Expected name 'test-gateway', got %s", fe.Name)
	}
	if fe.Mode != "http" {
		t.Errorf("Expected mode 'http', got %s", fe.Mode)
	}
	if fe.Options.MaxConnections != 10000 {
		t.Errorf("Expected max_connections 10000, got %d", fe.Options.MaxConnections)
	}
	if fe.Options.TimeoutClient != 30*time.Second {
		t.Errorf("Expected timeout_client 30s, got %v", fe.Options.TimeoutClient)
	}
}

func TestYAMLConfigProvider_InvalidFile(t *testing.T) {
	provider := NewYAMLConfigProvider("/nonexistent/file.yaml")

	_, err := provider.LoadConfig()
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestFlagConfigProvider_LoadConfig(t *testing.T) {
	args := utils.OSArgs{
		IPV4BindAddr:   "0.0.0.0",
		HTTPBindPort:   8080,
		HTTPSBindPort:  8443,
		DisableHTTP:    false,
		DisableHTTPS:   false,
		DisableIPV6:    true,
	}

	provider := NewFlagConfigProvider(args)
	config, err := provider.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if len(config.Frontends) != 1 {
		t.Errorf("Expected 1 frontend, got %d", len(config.Frontends))
	}

	fe := config.Frontends[0]
	if fe.ID != "default" {
		t.Errorf("Expected ID 'default', got %s", fe.ID)
	}

	if len(fe.Bindings) != 2 {
		t.Errorf("Expected 2 bindings (HTTP and HTTPS), got %d", len(fe.Bindings))
	}

	// Check HTTP binding
	httpBinding := fe.Bindings[0]
	if httpBinding.Port != 8080 {
		t.Errorf("Expected HTTP port 8080, got %d", httpBinding.Port)
	}
	if httpBinding.Protocol != "http" {
		t.Errorf("Expected HTTP protocol, got %s", httpBinding.Protocol)
	}

	// Check HTTPS binding
	httpsBinding := fe.Bindings[1]
	if httpsBinding.Port != 8443 {
		t.Errorf("Expected HTTPS port 8443, got %d", httpsBinding.Port)
	}
	if httpsBinding.Protocol != "https" {
		t.Errorf("Expected HTTPS protocol, got %s", httpsBinding.Protocol)
	}
	if !httpsBinding.SSL {
		t.Error("Expected SSL to be enabled for HTTPS binding")
	}
}

func TestFlagConfigProvider_DisabledProtocols(t *testing.T) {
	args := utils.OSArgs{
		DisableHTTP:  true,
		DisableHTTPS: true,
	}

	provider := NewFlagConfigProvider(args)
	_, err := provider.LoadConfig()
	if err == nil {
		t.Error("Expected error when both HTTP and HTTPS are disabled, got nil")
	}
}

func TestConfigRegistry_LoadConfig(t *testing.T) {
	// Create a temporary YAML config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "frontends.yaml")

	yamlContent := `frontends:
  - id: "yaml-frontend"
    name: "yaml-gateway"
    enabled: true
    mode: "http"
    bindings:
      - address: "0.0.0.0"
        port: 9090
        protocol: "http"
    routing:
      default_backend: "yaml-backend"
`

	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Create registry with YAML provider first, then flags
	registry := NewConfigRegistry()
	registry.Register(NewYAMLConfigProvider(configFile))
	registry.Register(NewFlagConfigProvider(utils.OSArgs{
		IPV4BindAddr: "0.0.0.0",
		HTTPBindPort: 8080,
	}))

	config, err := registry.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Should load from YAML (first provider)
	if config.Frontends[0].ID != "yaml-frontend" {
		t.Errorf("Expected YAML config to be loaded first, got ID: %s", config.Frontends[0].ID)
	}
}

func TestConfigRegistry_Fallback(t *testing.T) {
	// Create registry with invalid YAML provider, then valid flags provider
	registry := NewConfigRegistry()
	registry.Register(NewYAMLConfigProvider("/nonexistent/file.yaml"))
	registry.Register(NewFlagConfigProvider(utils.OSArgs{
		IPV4BindAddr:  "0.0.0.0",
		HTTPBindPort:  8080,
		DisableHTTPS:  true, // Disable HTTPS to avoid port 0 validation error
	}))

	config, err := registry.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Should fall back to flags provider
	if config.Frontends[0].ID != "default" {
		t.Errorf("Expected fallback to flags config, got ID: %s", config.Frontends[0].ID)
	}
}

func TestFrontendConfig_GetFrontendByID(t *testing.T) {
	config := FrontendConfig{
		Frontends: []FrontendDefinition{
			{ID: "frontend-1", Name: "gateway-1"},
			{ID: "frontend-2", Name: "gateway-2"},
		},
	}

	fe, err := config.GetFrontendByID("frontend-1")
	if err != nil {
		t.Errorf("GetFrontendByID() error = %v", err)
	}
	if fe.Name != "gateway-1" {
		t.Errorf("Expected name 'gateway-1', got %s", fe.Name)
	}

	_, err = config.GetFrontendByID("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent ID, got nil")
	}
}

func TestFrontendConfig_GetEnabledFrontends(t *testing.T) {
	config := FrontendConfig{
		Frontends: []FrontendDefinition{
			{ID: "frontend-1", Name: "gateway-1", Enabled: true},
			{ID: "frontend-2", Name: "gateway-2", Enabled: false},
			{ID: "frontend-3", Name: "gateway-3", Enabled: true},
		},
	}

	enabled := config.GetEnabledFrontends()
	if len(enabled) != 2 {
		t.Errorf("Expected 2 enabled frontends, got %d", len(enabled))
	}
}
