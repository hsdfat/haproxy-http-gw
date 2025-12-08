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
)

// ConfigProvider defines the interface for loading frontend configuration
// from different sources (YAML files, command-line flags, environment variables, etc.)
type ConfigProvider interface {
	// LoadConfig loads and returns the frontend configuration
	LoadConfig() (*FrontendConfig, error)

	// ValidateConfig validates the configuration
	ValidateConfig(*FrontendConfig) error

	// GetName returns the provider name for logging purposes
	GetName() string
}

// ConfigRegistry manages multiple config providers and tries them in priority order
type ConfigRegistry struct {
	providers []ConfigProvider
}

// NewConfigRegistry creates a new configuration registry
func NewConfigRegistry() *ConfigRegistry {
	return &ConfigRegistry{
		providers: make([]ConfigProvider, 0),
	}
}

// Register adds a provider to the registry
// Providers are tried in the order they are registered
func (r *ConfigRegistry) Register(provider ConfigProvider) {
	r.providers = append(r.providers, provider)
}

// RegisterMany adds multiple providers at once
func (r *ConfigRegistry) RegisterMany(providers ...ConfigProvider) {
	r.providers = append(r.providers, providers...)
}

// LoadConfig tries each provider in order until one succeeds
// Returns the first successful configuration or an error if all fail
func (r *ConfigRegistry) LoadConfig() (*FrontendConfig, error) {
	if len(r.providers) == 0 {
		return nil, fmt.Errorf("no config providers registered")
	}

	var lastErr error
	for _, provider := range r.providers {
		logger.Debugf("Trying config provider: %s", provider.GetName())

		config, err := provider.LoadConfig()
		if err != nil {
			logger.Debugf("Provider %s failed to load: %v", provider.GetName(), err)
			lastErr = err
			continue
		}

		// Validate the loaded config
		if err := provider.ValidateConfig(config); err != nil {
			logger.Debugf("Provider %s validation failed: %v", provider.GetName(), err)
			lastErr = err
			continue
		}

		// Apply defaults to the configuration
		for i := range config.Frontends {
			config.Frontends[i].SetDefaults()
		}

		logger.Infof("Configuration loaded successfully from: %s", provider.GetName())
		logger.Infof("Loaded %d frontend(s)", len(config.Frontends))
		return config, nil
	}

	return nil, fmt.Errorf("all config providers failed, last error: %w", lastErr)
}

// GetProviders returns the list of registered providers
func (r *ConfigRegistry) GetProviders() []ConfigProvider {
	return r.providers
}

// Clear removes all registered providers
func (r *ConfigRegistry) Clear() {
	r.providers = make([]ConfigProvider, 0)
}
