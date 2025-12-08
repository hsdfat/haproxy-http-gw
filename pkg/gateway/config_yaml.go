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
	"os"

	"gopkg.in/yaml.v3"
)

// YAMLConfigProvider loads frontend configuration from a YAML file
type YAMLConfigProvider struct {
	FilePath string
}

// NewYAMLConfigProvider creates a new YAML configuration provider
func NewYAMLConfigProvider(filePath string) *YAMLConfigProvider {
	return &YAMLConfigProvider{
		FilePath: filePath,
	}
}

// GetName returns the provider name
func (p *YAMLConfigProvider) GetName() string {
	return fmt.Sprintf("YAML(%s)", p.FilePath)
}

// LoadConfig loads configuration from the YAML file
func (p *YAMLConfigProvider) LoadConfig() (*FrontendConfig, error) {
	if p.FilePath == "" {
		return nil, fmt.Errorf("YAML file path not specified")
	}

	// Check if file exists
	if _, err := os.Stat(p.FilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", p.FilePath)
	}

	// Read file
	data, err := os.ReadFile(p.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config FrontendConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &config, nil
}

// ValidateConfig validates the loaded configuration
func (p *YAMLConfigProvider) ValidateConfig(config *FrontendConfig) error {
	if config == nil {
		return fmt.Errorf("config is nil")
	}
	return config.Validate()
}
