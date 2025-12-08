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

	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

// FlagConfigProvider converts command-line flags to FrontendConfig
// This provides backward compatibility with existing flag-based configuration
type FlagConfigProvider struct {
	OSArgs utils.OSArgs
}

// NewFlagConfigProvider creates a new flags configuration provider
func NewFlagConfigProvider(args utils.OSArgs) *FlagConfigProvider {
	return &FlagConfigProvider{
		OSArgs: args,
	}
}

// GetName returns the provider name
func (p *FlagConfigProvider) GetName() string {
	return "Flags"
}

// LoadConfig converts command-line flags to FrontendConfig
func (p *FlagConfigProvider) LoadConfig() (*FrontendConfig, error) {
	// Create a single frontend from existing flags for backward compatibility
	frontend := FrontendDefinition{
		ID:      "default",
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
		httpBinding := BindingDefinition{
			Address:  p.OSArgs.IPV4BindAddr,
			Port:     int(p.OSArgs.HTTPBindPort),
			Protocol: "http",
			HTTP2:    true,
			IPv6:     !p.OSArgs.DisableIPV6,
		}
		frontend.Bindings = append(frontend.Bindings, httpBinding)
	}

	// Add HTTPS binding if not disabled
	if !p.OSArgs.DisableHTTPS {
		httpsBinding := BindingDefinition{
			Address:  p.OSArgs.IPV4BindAddr,
			Port:     int(p.OSArgs.HTTPSBindPort),
			Protocol: "https",
			SSL:      true,
			HTTP2:    true,
			ALPN:     "h2,http/1.1",
			IPv6:     !p.OSArgs.DisableIPV6,
		}
		frontend.Bindings = append(frontend.Bindings, httpsBinding)
	}

	// Validate that at least one binding is configured
	if len(frontend.Bindings) == 0 {
		return nil, fmt.Errorf("no bindings configured (both HTTP and HTTPS disabled)")
	}

	// Set default backend from flags if specified
	if p.OSArgs.DefaultBackendService.String() != "" {
		frontend.Routing.DefaultBackend = p.OSArgs.DefaultBackendService.String()
	}

	config := &FrontendConfig{
		Frontends: []FrontendDefinition{frontend},
	}

	return config, nil
}

// ValidateConfig validates the configuration
func (p *FlagConfigProvider) ValidateConfig(config *FrontendConfig) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}
	return config.Validate()
}
