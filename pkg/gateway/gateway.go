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
	"fmt"
	"time"

	"github.com/haproxytech/client-native/v6/models"
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
)

// HTTPGateway represents an HTTP/HTTP2 gateway
type HTTPGateway struct {
	haproxyClient api.HAProxyClient
	manager       *Manager
	config        GatewayConfig
}

// GatewayConfig holds configuration for the HTTP Gateway
type GatewayConfig struct {
	// Frontend configuration
	FrontendName string
	HTTPPort     int
	HTTPSPort    int
	HTTPSEnabled bool

	// SSL/TLS configuration
	SSLCertDir string
	StrictSNI  bool

	// HTTP/2 configuration
	EnableHTTP2 bool
	ALPN        string // e.g., "h2,http/1.1"

	// Default backend
	DefaultBackend string

	// IPv4 and IPv6 bind addresses
	IPv4BindAddr string
	IPv6BindAddr string
}

// NewHTTPGateway creates a new HTTP gateway
func NewHTTPGateway(haproxyClient api.HAProxyClient, manager *Manager, config GatewayConfig) *HTTPGateway {
	// Set defaults
	if config.FrontendName == "" {
		config.FrontendName = "http-gateway"
	}
	if config.HTTPPort == 0 {
		config.HTTPPort = 80
	}
	if config.HTTPSPort == 0 {
		config.HTTPSPort = 443
	}
	if config.ALPN == "" {
		config.ALPN = "h2,http/1.1"
	}
	if config.IPv4BindAddr == "" {
		config.IPv4BindAddr = "0.0.0.0"
	}
	if config.IPv6BindAddr == "" {
		config.IPv6BindAddr = "::"
	}

	return &HTTPGateway{
		haproxyClient: haproxyClient,
		manager:       manager,
		config:        config,
	}
}

// Start initializes the HTTP gateway and configures HAProxy
func (g *HTTPGateway) Start(ctx context.Context) error {
	logger.Info("Starting HTTP Gateway")

	// Start the backend manager first to create backends
	if err := g.manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	// Wait a moment for backends to be created by the provider
	// This ensures the default backend exists before frontend validation
	logger.Info("Waiting for initial backend synchronization...")
	time.Sleep(2 * time.Second)

	// Configure HAProxy frontend (after backends exist)
	if err := g.configureFrontend(); err != nil {
		return fmt.Errorf("failed to configure frontend: %w", err)
	}

	logger.Infof("HTTP Gateway started on HTTP:%d HTTPS:%d with HTTP/2 support", g.config.HTTPPort, g.config.HTTPSPort)
	return nil
}

// StartWithoutFrontend starts the gateway without configuring the frontend
// This is used when the frontend is statically configured in haproxy.cfg
func (g *HTTPGateway) StartWithoutFrontend(ctx context.Context) error {
	logger.Info("Starting HTTP Gateway (frontend pre-configured)")

	// Start the backend manager
	if err := g.manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	logger.Infof("HTTP Gateway started with pre-configured frontend on HTTP:%d", g.config.HTTPPort)
	logger.Info("Backends will be registered dynamically via API")
	return nil
}

// Stop stops the HTTP gateway
func (g *HTTPGateway) Stop() error {
	logger.Info("Stopping HTTP Gateway")
	return g.manager.Stop()
}

// configureFrontend sets up the HAProxy frontend with HTTP/2 support
func (g *HTTPGateway) configureFrontend() error {
	logger.Info("Configuring HAProxy frontend")

	if err := g.haproxyClient.APIStartTransaction(); err != nil {
		return err
	}
	defer g.haproxyClient.APIDisposeTransaction()

	// Create frontend
	frontend := models.FrontendBase{
		Name: g.config.FrontendName,
		Mode: "http",
		DefaultBackend: g.config.DefaultBackend,
		// Enable HTTP/2
		HTTPConnectionMode: "http-keep-alive",
	}

	if err := g.haproxyClient.FrontendCreate(frontend); err != nil {
		// Frontend might already exist, try to edit it
		if err := g.haproxyClient.FrontendEdit(frontend); err != nil {
			logger.Debugf("Frontend edit failed (might be ok): %v", err)
		}
	}

	// Configure HTTP binding with H2C support (HTTP/2 Cleartext)
	httpBind := models.Bind{
		BindParams: models.BindParams{
			Name: "http-ipv4",
		},
		Address: fmt.Sprintf("%s:%d", g.config.IPv4BindAddr, g.config.HTTPPort),
	}

	// Add H2C (HTTP/2 Cleartext) support with HTTP/1.1 fallback if HTTP/2 is enabled
	if g.config.EnableHTTP2 {
		httpBind.BindParams.Proto = "h2,http/1.1"
	}

	if err := g.haproxyClient.FrontendBindCreate(g.config.FrontendName, httpBind); err != nil {
		logger.Debugf("HTTP bind creation failed (might already exist): %v", err)
	}

	// Configure HTTPS binding with HTTP/2 support
	if g.config.HTTPSEnabled {
		httpsBind := models.Bind{
			BindParams: models.BindParams{
				Name: "https-ipv4",
				Ssl:  true,
				// Enable HTTP/2 via ALPN
				Alpn: g.config.ALPN,
			},
			Address: fmt.Sprintf("%s:%d", g.config.IPv4BindAddr, g.config.HTTPSPort),
		}

		if g.config.SSLCertDir != "" {
			httpsBind.SslCertificate = g.config.SSLCertDir
		}

		if err := g.haproxyClient.FrontendBindCreate(g.config.FrontendName, httpsBind); err != nil {
			logger.Debugf("HTTPS bind creation failed (might already exist): %v", err)
		}
	}

	// Configure IPv6 bindings if needed with H2C (HTTP/2 Cleartext) support
	httpBindV6 := models.Bind{
		BindParams: models.BindParams{
			Name: "http-ipv6",
			V4v6: true,
		},
		Address: fmt.Sprintf("[%s]:%d", g.config.IPv6BindAddr, g.config.HTTPPort),
	}

	// Add H2C support if HTTP/2 is enabled
	if g.config.EnableHTTP2 {
		httpBindV6.BindParams.Proto = "h2,http/1.1"
	}

	if err := g.haproxyClient.FrontendBindCreate(g.config.FrontendName, httpBindV6); err != nil {
		logger.Debugf("HTTP IPv6 bind creation failed (might already exist): %v", err)
	}

	if g.config.HTTPSEnabled {
		httpsBindV6 := models.Bind{
			BindParams: models.BindParams{
				Name: "https-ipv6",
				V4v6: true,
				Ssl:  true,
				Alpn: g.config.ALPN,
			},
			Address: fmt.Sprintf("[%s]:%d", g.config.IPv6BindAddr, g.config.HTTPSPort),
		}

		if g.config.SSLCertDir != "" {
			httpsBindV6.SslCertificate = g.config.SSLCertDir
		}

		if err := g.haproxyClient.FrontendBindCreate(g.config.FrontendName, httpsBindV6); err != nil {
			logger.Debugf("HTTPS IPv6 bind creation failed (might already exist): %v", err)
		}
	}

	// Final commit (handles both frontend processing and transaction commit)
	if err := g.haproxyClient.APIFinalCommitTransaction(); err != nil {
		return err
	}

	logger.Info("Frontend configuration complete")
	return nil
}

// AddBackendRoute adds a routing rule to direct traffic to a specific backend
// based on host/path matching
func (g *HTTPGateway) AddBackendRoute(host, path, backendName string) error {
	logger.Infof("Adding route: host=%s path=%s -> backend=%s", host, path, backendName)

	// Build ACLs and condition based on host and path
	var condTest string
	var aclsToAdd []*models.ACL

	if host != "" && path != "" {
		// Need both host and path ACLs
		hostACLName := fmt.Sprintf("host_%s", sanitizeName(host))
		pathACLName := fmt.Sprintf("path_%s", sanitizeName(path))

		aclsToAdd = append(aclsToAdd, &models.ACL{
			ACLName:   hostACLName,
			Criterion: "hdr(host)",
			Value:     fmt.Sprintf("-i %s", host),
		})
		aclsToAdd = append(aclsToAdd, &models.ACL{
			ACLName:   pathACLName,
			Criterion: "path_beg",
			Value:     path,
		})

		condTest = fmt.Sprintf("%s %s", hostACLName, pathACLName)
	} else if host != "" {
		aclName := fmt.Sprintf("host_%s", sanitizeName(host))
		aclsToAdd = append(aclsToAdd, &models.ACL{
			ACLName:   aclName,
			Criterion: "hdr(host)",
			Value:     fmt.Sprintf("-i %s", host),
		})
		condTest = aclName
	} else if path != "" {
		aclName := fmt.Sprintf("path_%s", sanitizeName(path))
		aclsToAdd = append(aclsToAdd, &models.ACL{
			ACLName:   aclName,
			Criterion: "path_beg",
			Value:     path,
		})
		condTest = aclName
	} else {
		return fmt.Errorf("either host or path must be specified")
	}

	// Start transaction
	if err := g.haproxyClient.APIStartTransaction(); err != nil {
		return err
	}

	// Get existing ACLs from the frontend within the transaction
	existingACLs, err := g.haproxyClient.ACLsGet("frontend", g.config.FrontendName)
	if err != nil {
		logger.Debugf("Could not fetch existing ACLs (may not exist yet): %v", err)
		existingACLs = models.Acls{}
	}

	// Merge new ACLs with existing ones (avoid duplicates based on ACL name)
	aclMap := make(map[string]*models.ACL)
	for i := range existingACLs {
		aclMap[existingACLs[i].ACLName] = existingACLs[i]
	}
	for _, newACL := range aclsToAdd {
		aclMap[newACL.ACLName] = newACL
	}

	// Convert map back to slice
	allACLs := make(models.Acls, 0, len(aclMap))
	for _, acl := range aclMap {
		allACLs = append(allACLs, acl)
	}

	// Replace all ACLs (this handles both creating new ones and preserving existing ones)
	if err := g.haproxyClient.ACLsReplace("frontend", g.config.FrontendName, allACLs); err != nil {
		g.haproxyClient.APIDisposeTransaction()
		return fmt.Errorf("failed to update ACLs: %w", err)
	}

	// Get existing backend switching rules to find the next index within the transaction
	existingRules, err := g.haproxyClient.BackendSwitchingRulesGet(g.config.FrontendName)
	if err != nil {
		logger.Debugf("Could not fetch existing rules (may not exist yet): %v", err)
		existingRules = models.BackendSwitchingRules{}
	}
	nextIndex := int64(len(existingRules))

	// Create backend switching rule
	rule := models.BackendSwitchingRule{
		Cond:     "if",
		CondTest: condTest,
		Name:     backendName,
	}

	if err := g.haproxyClient.BackendSwitchingRuleCreate(nextIndex, g.config.FrontendName, rule); err != nil {
		g.haproxyClient.APIDisposeTransaction()
		return fmt.Errorf("failed to create backend switching rule: %w", err)
	}

	// Final commit (processes changes and commits the transaction)
	// Note: APIFinalCommitTransaction handles both processing and transaction commit
	if err := g.haproxyClient.APIFinalCommitTransaction(); err != nil {
		return fmt.Errorf("failed to apply configuration: %w", err)
	}

	logger.Infof("Route added successfully")
	return nil
}

// sanitizeName removes special characters from names
func sanitizeName(name string) string {
	result := ""
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			result += string(c)
		} else {
			result += "_"
		}
	}
	return result
}

// RegisterBackend registers a new backend with the gateway
func (g *HTTPGateway) RegisterBackend(backend Backend) error {
	logger.Infof("Registering backend: %s with %d servers", backend.Name, len(backend.Servers))
	return g.manager.RegisterBackend(backend)
}

// UnregisterBackend removes a backend from the gateway
func (g *HTTPGateway) UnregisterBackend(name string) error {
	logger.Infof("Unregistering backend: %s", name)
	return g.manager.UnregisterBackend(name)
}

// GetBackends returns all registered backends
func (g *HTTPGateway) GetBackends() map[string]*Backend {
	return g.manager.GetBackends()
}
