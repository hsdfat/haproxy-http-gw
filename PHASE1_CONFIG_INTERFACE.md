# Phase 1: Configuration Interface Design

## Overview

Phase 1 focuses on building a **flexible configuration infrastructure** using the **interface pattern** to support multiple configuration sources. This enables the system to load frontend configuration from YAML files, command-line flags, environment variables, or any custom source.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                    ConfigRegistry                            │
│  (Tries providers in order until one succeeds)              │
└────────────┬────────────────────────────────────────────────┘
             │
             │ Implements ConfigProvider interface
             │
    ┌────────┴────────┬─────────────┬──────────────────┐
    │                 │             │                  │
    v                 v             v                  v
┌──────────┐  ┌──────────────┐  ┌────────┐  ┌──────────────┐
│  YAML    │  │    Flags     │  │  Env   │  │   Custom     │
│ Provider │  │  Provider    │  │Provider│  │  Provider    │
└──────────┘  └──────────────┘  └────────┘  └──────────────┘
     │               │              │              │
     └───────────────┴──────────────┴──────────────┘
                     │
                     v
            ┌─────────────────┐
            │ FrontendConfig  │
            │  - Frontends[]  │
            └─────────────────┘
                     │
                     v
         ┌───────────────────────┐
         │  FrontendDefinition   │
         │  - ID                 │
         │  - Name               │
         │  - Bindings[]         │
         │  - Routing            │
         │  - Options            │
         └───────────────────────┘
```

## ConfigProvider Interface

The core abstraction that enables multiple configuration sources:

```go
package gateway

// ConfigProvider defines the interface for loading frontend configuration
// from different sources (YAML, flags, environment, etc.)
type ConfigProvider interface {
    // LoadConfig loads and returns the frontend configuration
    LoadConfig() (*FrontendConfig, error)

    // ValidateConfig validates the configuration
    ValidateConfig(*FrontendConfig) error

    // GetName returns the provider name for logging
    GetName() string
}
```

## Configuration Flow

```
1. Application Start
        │
        v
2. Create ConfigRegistry
        │
        v
3. Register Providers (in priority order)
   - YAMLConfigProvider (highest priority)
   - FlagConfigProvider (backward compatibility)
   - EnvConfigProvider (fallback)
        │
        v
4. registry.LoadConfig()
   - Try YAML provider
   - If fails, try Flags provider
   - If fails, try Env provider
   - If all fail, return error
        │
        v
5. Validate Configuration
        │
        v
6. Return FrontendConfig
        │
        v
7. Pass to FrontendManager
```

## Implementation Details

### 1. Configuration Types

```go
package gateway

import "time"

// FrontendConfig is the root configuration structure
type FrontendConfig struct {
    Frontends []FrontendDefinition `yaml:"frontends" json:"frontends"`
}

// FrontendDefinition defines a single frontend
type FrontendDefinition struct {
    ID       string              `yaml:"id" json:"id"`
    Name     string              `yaml:"name" json:"name"`
    Enabled  bool                `yaml:"enabled" json:"enabled"`
    Mode     string              `yaml:"mode" json:"mode"` // "http" or "tcp"
    Bindings []BindingDefinition `yaml:"bindings" json:"bindings"`
    Routing  RoutingConfig       `yaml:"routing" json:"routing"`
    Options  FrontendOptions     `yaml:"options" json:"options"`
}

// BindingDefinition defines how a frontend binds to network
type BindingDefinition struct {
    Address  string `yaml:"address" json:"address"`
    Port     int    `yaml:"port" json:"port"`
    Protocol string `yaml:"protocol" json:"protocol"` // "http", "https", "tcp"
    HTTP2    bool   `yaml:"http2" json:"http2"`
    SSL      bool   `yaml:"ssl" json:"ssl"`
    ALPN     string `yaml:"alpn" json:"alpn"`
    CertDir  string `yaml:"cert_dir" json:"cert_dir"`
    IPv6     bool   `yaml:"ipv6" json:"ipv6"`
}

// RoutingConfig defines routing behavior
type RoutingConfig struct {
    BypassRules    bool   `yaml:"bypass_rules" json:"bypass_rules"`
    DefaultBackend string `yaml:"default_backend" json:"default_backend"`
}

// FrontendOptions holds additional frontend options
type FrontendOptions struct {
    MaxConnections     int           `yaml:"max_connections" json:"max_connections"`
    TimeoutClient      time.Duration `yaml:"timeout_client" json:"timeout_client"`
    HTTPConnectionMode string        `yaml:"http_connection_mode" json:"http_connection_mode"`
}

// Validate validates the frontend configuration
func (fc *FrontendConfig) Validate() error {
    if len(fc.Frontends) == 0 {
        return fmt.Errorf("at least one frontend must be defined")
    }

    // Check for duplicate IDs
    ids := make(map[string]bool)
    names := make(map[string]bool)

    for _, fe := range fc.Frontends {
        if err := fe.Validate(); err != nil {
            return fmt.Errorf("frontend %s: %w", fe.ID, err)
        }

        if ids[fe.ID] {
            return fmt.Errorf("duplicate frontend ID: %s", fe.ID)
        }
        ids[fe.ID] = true

        if names[fe.Name] {
            return fmt.Errorf("duplicate frontend name: %s", fe.Name)
        }
        names[fe.Name] = true
    }

    return nil
}

// Validate validates a single frontend definition
func (fd *FrontendDefinition) Validate() error {
    if fd.ID == "" {
        return fmt.Errorf("frontend ID is required")
    }
    if fd.Name == "" {
        return fmt.Errorf("frontend name is required")
    }
    if fd.Mode != "http" && fd.Mode != "tcp" {
        return fmt.Errorf("invalid mode: %s (must be 'http' or 'tcp')", fd.Mode)
    }
    if len(fd.Bindings) == 0 {
        return fmt.Errorf("at least one binding is required")
    }

    for i, binding := range fd.Bindings {
        if err := binding.Validate(); err != nil {
            return fmt.Errorf("binding %d: %w", i, err)
        }
    }

    if fd.Routing.DefaultBackend == "" {
        return fmt.Errorf("default_backend is required")
    }

    return nil
}

// Validate validates a binding definition
func (bd *BindingDefinition) Validate() error {
    if bd.Address == "" {
        return fmt.Errorf("binding address is required")
    }
    if bd.Port <= 0 || bd.Port > 65535 {
        return fmt.Errorf("invalid port: %d", bd.Port)
    }
    if bd.Protocol != "http" && bd.Protocol != "https" && bd.Protocol != "tcp" {
        return fmt.Errorf("invalid protocol: %s", bd.Protocol)
    }
    return nil
}
```

### 2. YAML Config Provider

```go
package gateway

import (
    "fmt"
    "os"

    "gopkg.in/yaml.v3"
)

// YAMLConfigProvider loads configuration from a YAML file
type YAMLConfigProvider struct {
    FilePath string
}

// NewYAMLConfigProvider creates a new YAML config provider
func NewYAMLConfigProvider(filePath string) *YAMLConfigProvider {
    return &YAMLConfigProvider{
        FilePath: filePath,
    }
}

// GetName returns the provider name
func (p *YAMLConfigProvider) GetName() string {
    return "YAML"
}

// LoadConfig loads configuration from YAML file
func (p *YAMLConfigProvider) LoadConfig() (*FrontendConfig, error) {
    if p.FilePath == "" {
        return nil, fmt.Errorf("YAML file path not specified")
    }

    data, err := os.ReadFile(p.FilePath)
    if err != nil {
        return nil, fmt.Errorf("failed to read config file: %w", err)
    }

    var config FrontendConfig
    if err := yaml.Unmarshal(data, &config); err != nil {
        return nil, fmt.Errorf("failed to parse YAML: %w", err)
    }

    return &config, nil
}

// ValidateConfig validates the configuration
func (p *YAMLConfigProvider) ValidateConfig(config *FrontendConfig) error {
    return config.Validate()
}
```

### 3. Flags Config Provider

```go
package gateway

import (
    "fmt"

    "github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

// FlagConfigProvider converts command-line flags to FrontendConfig
// This provides backward compatibility with existing flag-based configuration
type FlagConfigProvider struct {
    OSArgs utils.OSArgs
}

// NewFlagConfigProvider creates a new flags config provider
func NewFlagConfigProvider(args utils.OSArgs) *FlagConfigProvider {
    return &FlagConfigProvider{
        OSArgs: args,
    }
}

// GetName returns the provider name
func (p *FlagConfigProvider) GetName() string {
    return "Flags"
}

// LoadConfig converts flags to FrontendConfig
func (p *FlagConfigProvider) LoadConfig() (*FrontendConfig, error) {
    // Create a single frontend from existing flags
    frontend := FrontendDefinition{
        ID:      "default-frontend",
        Name:    "http-gateway",
        Enabled: true,
        Mode:    "http",
        Bindings: []BindingDefinition{},
        Routing: RoutingConfig{
            BypassRules:    false,
            DefaultBackend: "default-backend",
        },
        Options: FrontendOptions{
            HTTPConnectionMode: "http-keep-alive",
        },
    }

    // Add HTTP binding if not disabled
    if !p.OSArgs.DisableHTTP {
        frontend.Bindings = append(frontend.Bindings, BindingDefinition{
            Address:  p.OSArgs.IPV4BindAddr,
            Port:     int(p.OSArgs.HTTPBindPort),
            Protocol: "http",
            HTTP2:    true,
            IPv6:     !p.OSArgs.DisableIPV6,
        })
    }

    // Add HTTPS binding if not disabled
    if !p.OSArgs.DisableHTTPS {
        frontend.Bindings = append(frontend.Bindings, BindingDefinition{
            Address:  p.OSArgs.IPV4BindAddr,
            Port:     int(p.OSArgs.HTTPSBindPort),
            Protocol: "https",
            SSL:      true,
            HTTP2:    true,
            ALPN:     "h2,http/1.1",
            IPv6:     !p.OSArgs.DisableIPV6,
        })
    }

    if len(frontend.Bindings) == 0 {
        return nil, fmt.Errorf("no bindings configured (both HTTP and HTTPS disabled)")
    }

    config := &FrontendConfig{
        Frontends: []FrontendDefinition{frontend},
    }

    return config, nil
}

// ValidateConfig validates the configuration
func (p *FlagConfigProvider) ValidateConfig(config *FrontendConfig) error {
    return config.Validate()
}
```

### 4. Config Registry

```go
package gateway

import (
    "fmt"
)

// ConfigRegistry manages multiple config providers and tries them in order
type ConfigRegistry struct {
    providers []ConfigProvider
}

// NewConfigRegistry creates a new config registry
func NewConfigRegistry() *ConfigRegistry {
    return &ConfigRegistry{
        providers: []ConfigProvider{},
    }
}

// Register adds a provider to the registry
func (r *ConfigRegistry) Register(provider ConfigProvider) {
    r.providers = append(r.providers, provider)
}

// LoadConfig tries each provider in order until one succeeds
func (r *ConfigRegistry) LoadConfig() (*FrontendConfig, error) {
    if len(r.providers) == 0 {
        return nil, fmt.Errorf("no config providers registered")
    }

    var lastErr error
    for _, provider := range r.providers {
        logger.Debugf("Trying config provider: %s", provider.GetName())

        config, err := provider.LoadConfig()
        if err != nil {
            logger.Debugf("Provider %s failed: %v", provider.GetName(), err)
            lastErr = err
            continue
        }

        // Validate the loaded config
        if err := provider.ValidateConfig(config); err != nil {
            logger.Debugf("Provider %s validation failed: %v", provider.GetName(), err)
            lastErr = err
            continue
        }

        logger.Infof("Configuration loaded successfully from: %s", provider.GetName())
        return config, nil
    }

    return nil, fmt.Errorf("all config providers failed, last error: %w", lastErr)
}
```

## Usage Example

```go
package main

import (
    "github.com/haproxytech/kubernetes-ingress/pkg/gateway"
    "github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

func main() {
    var osArgs utils.OSArgs
    // ... parse flags ...

    // Create config registry
    registry := gateway.NewConfigRegistry()

    // Register providers in priority order
    // 1. Try YAML file first (if specified)
    if osArgs.FrontendConfigFile != "" {
        yamlProvider := gateway.NewYAMLConfigProvider(osArgs.FrontendConfigFile)
        registry.Register(yamlProvider)
    }

    // 2. Fall back to flags (backward compatibility)
    flagProvider := gateway.NewFlagConfigProvider(osArgs)
    registry.Register(flagProvider)

    // Load configuration (tries providers in order)
    config, err := registry.LoadConfig()
    if err != nil {
        logger.Fatalf("Failed to load configuration: %v", err)
    }

    // Use the configuration
    frontendManager := gateway.NewFrontendManager(haproxyClient, *config)
    frontendManager.Start(ctx)
}
```

## Benefits of Interface Design

1. **Extensibility**: Easy to add new config sources (database, HTTP API, etc.)
2. **Testability**: Can create mock providers for testing
3. **Backward Compatibility**: Flags provider enables gradual migration
4. **Flexibility**: Users choose their preferred config method
5. **Validation**: Centralized validation logic
6. **Priority**: Registry allows fallback chain

## Testing Strategy

### Unit Tests

```go
func TestYAMLConfigProvider_LoadConfig(t *testing.T) {
    // Test valid YAML
    // Test invalid YAML
    // Test missing file
    // Test validation errors
}

func TestFlagConfigProvider_LoadConfig(t *testing.T) {
    // Test flag conversion
    // Test disabled HTTP/HTTPS
    // Test IPv6 settings
}

func TestConfigRegistry_LoadConfig(t *testing.T) {
    // Test provider priority
    // Test fallback on failure
    // Test all providers fail
}

func TestFrontendConfig_Validate(t *testing.T) {
    // Test valid config
    // Test duplicate IDs
    // Test duplicate names
    // Test missing required fields
}
```

### Integration Tests

```go
func TestConfigIntegration(t *testing.T) {
    // Test loading from YAML file
    // Test creating FrontendManager with config
    // Test multiple frontends configuration
}
```

## Phase 1 Deliverables Checklist

- [ ] `pkg/gateway/config.go` - Configuration types with validation
- [ ] `pkg/gateway/config_provider.go` - ConfigProvider interface and registry
- [ ] `pkg/gateway/config_yaml.go` - YAML provider implementation
- [ ] `pkg/gateway/config_flags.go` - Flags provider implementation
- [ ] `pkg/gateway/config_test.go` - Unit tests for all components
- [ ] `pkg/utils/flags.go` - Add `FrontendConfigFile` flag
- [ ] Example YAML configurations in `examples/`
- [ ] Documentation updates

## Next Phase

After Phase 1 is complete, Phase 2 will use the `FrontendConfig` to create and manage actual HAProxy frontends.

```go
// Phase 2 usage
config, _ := registry.LoadConfig()
manager := gateway.NewFrontendManager(haproxyClient, *config)
manager.Start(ctx)
```
