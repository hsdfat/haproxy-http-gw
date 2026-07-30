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
	"sync"
	"time"

	"github.com/haproxytech/client-native/v6/models"
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
)

// FrontendManager manages multiple frontends based on configuration
type FrontendManager struct {
	haproxyClient api.HAProxyClient
	config        FrontendConfig
	frontends     map[string]*ManagedFrontend
	mu            sync.RWMutex
}

// ManagedFrontend represents a managed frontend instance
type ManagedFrontend struct {
	Definition FrontendDefinition
	Manager    *Manager     // Backend manager for this frontend
	Routes     map[string]Route // Routing rules for this frontend
	mu         sync.RWMutex
}

// Route represents a routing rule in a frontend
type Route struct {
	ID          string // Unique route ID
	Host        string // Host pattern
	Path        string // Path pattern
	BackendName string // Target backend
	FrontendID  string // Frontend this route belongs to
}

// NewFrontendManager creates a new frontend manager
func NewFrontendManager(haproxyClient api.HAProxyClient, config FrontendConfig) *FrontendManager {
	return &FrontendManager{
		haproxyClient: haproxyClient,
		config:        config,
		frontends:     make(map[string]*ManagedFrontend),
	}
}

// Start initializes all configured frontends
func (fm *FrontendManager) Start(ctx context.Context) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	logger.Info("Starting Frontend Manager")

	for _, frontendDef := range fm.config.Frontends {
		if !frontendDef.Enabled {
			logger.Infof("Skipping disabled frontend: %s", frontendDef.ID)
			continue
		}

		if err := fm.startFrontend(ctx, frontendDef); err != nil {
			return fmt.Errorf("failed to start frontend %s: %w", frontendDef.ID, err)
		}
	}

	logger.Infof("Frontend Manager started with %d frontend(s)", len(fm.frontends))
	return nil
}

// startFrontend initializes a single frontend
func (fm *FrontendManager) startFrontend(ctx context.Context, def FrontendDefinition) error {
	logger.Infof("Starting frontend: %s (name=%s)", def.ID, def.Name)

	// Create backend manager for this frontend
	backendMgr := NewManager(ManagerConfig{
		HAProxyClient: fm.haproxyClient,
		Provider:      nil, // Provider can be set later if needed
		SyncPeriod:    5 * time.Second,
	})

	// Create managed frontend
	mf := &ManagedFrontend{
		Definition: def,
		Manager:    backendMgr,
		Routes:     make(map[string]Route),
	}

	// Configure HAProxy frontend
	if err := fm.configureFrontend(def); err != nil {
		return fmt.Errorf("failed to configure frontend: %w", err)
	}

	// Start backend manager
	if err := backendMgr.Start(ctx); err != nil {
		return fmt.Errorf("failed to start backend manager: %w", err)
	}

	fm.frontends[def.ID] = mf
	logger.Infof("Frontend %s started successfully", def.ID)

	return nil
}

// configureFrontend creates HAProxy frontend configuration
func (fm *FrontendManager) configureFrontend(def FrontendDefinition) error {
	logger.Infof("Configuring HAProxy frontend: %s", def.Name)

	if err := fm.haproxyClient.APIStartTransaction(); err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer fm.haproxyClient.APIDisposeTransaction()

	// Create frontend base configuration
	frontend := models.FrontendBase{
		Name:           def.Name,
		Mode:           def.Mode,
		DefaultBackend: def.Routing.DefaultBackend,
	}

	// Set HTTP connection mode
	if def.Options.HTTPConnectionMode != "" {
		frontend.HTTPConnectionMode = def.Options.HTTPConnectionMode
	}

	// Set max connections
	if def.Options.MaxConnections > 0 {
		maxconn := int64(def.Options.MaxConnections)
		frontend.Maxconn = &maxconn
	}

	// Create or update frontend
	if err := fm.haproxyClient.FrontendCreate(frontend); err != nil {
		// Frontend might already exist, try to edit it
		if err := fm.haproxyClient.FrontendEdit(frontend); err != nil {
			logger.Debugf("Frontend edit failed (might be ok): %v", err)
		}
	}

	// Configure bindings
	for i, binding := range def.Bindings {
		if err := fm.createBinding(def.Name, binding, i); err != nil {
			logger.Errorf("Failed to create binding %d for frontend %s: %v", i, def.Name, err)
			// Continue with other bindings even if one fails
		}
	}

	// Commit transaction
	if err := fm.haproxyClient.APIFinalCommitTransaction(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Infof("Frontend %s configured successfully", def.Name)
	return nil
}

// createBinding creates a single bind configuration
func (fm *FrontendManager) createBinding(frontendName string, binding BindingDefinition, index int) error {
	bind := models.Bind{
		BindParams: models.BindParams{
			Name: fmt.Sprintf("bind-%d", index),
		},
		Address: fmt.Sprintf("%s:%d", binding.Address, binding.Port),
	}

	// Configure SSL
	if binding.SSL {
		bind.BindParams.Ssl = true
		if binding.ALPN != "" {
			bind.BindParams.Alpn = binding.ALPN
		}
		if binding.CertDir != "" {
			bind.SslCertificate = binding.CertDir
		}
	}

	// Configure HTTP/2
	if binding.HTTP2 && binding.Protocol == "http" {
		bind.BindParams.Proto = "h2"
	}

	// Configure IPv6
	if binding.IPv6 {
		bind.BindParams.V4v6 = true
	}

	logger.Debugf("Creating binding %s for frontend %s", bind.Address, frontendName)
	return fm.haproxyClient.FrontendBindCreate(frontendName, bind)
}

// RegisterBackend registers a backend with a specific frontend
func (fm *FrontendManager) RegisterBackend(frontendID string, backend Backend) error {
	fm.mu.RLock()
	mf, exists := fm.frontends[frontendID]
	fm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("frontend %s not found", frontendID)
	}

	logger.Infof("Registering backend %s to frontend %s", backend.Name, frontendID)
	return mf.Manager.RegisterBackend(backend)
}

// UnregisterBackend removes a backend from a specific frontend
func (fm *FrontendManager) UnregisterBackend(frontendID string, backendName string) error {
	fm.mu.RLock()
	mf, exists := fm.frontends[frontendID]
	fm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("frontend %s not found", frontendID)
	}

	logger.Infof("Unregistering backend %s from frontend %s", backendName, frontendID)
	return mf.Manager.UnregisterBackend(backendName)
}

// AddRoute adds a routing rule to a specific frontend
func (fm *FrontendManager) AddRoute(frontendID string, route Route) error {
	fm.mu.RLock()
	mf, exists := fm.frontends[frontendID]
	fm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("frontend %s not found", frontendID)
	}

	// Check if bypass mode is enabled (Phase 4 feature - not enforced yet)
	if mf.Definition.Routing.BypassRules {
		logger.Warningf("Frontend %s has bypass_rules enabled, but bypass mode is not yet implemented (Phase 4). Route will be added anyway.", frontendID)
		// In Phase 4, this will return an error:
		// return fmt.Errorf("frontend %s has bypass_rules enabled, routing not allowed", frontendID)
	}

	mf.mu.Lock()
	defer mf.mu.Unlock()

	// Add route configuration to HAProxy
	if err := fm.addRouteToHAProxy(mf.Definition.Name, route); err != nil {
		return fmt.Errorf("failed to add route to HAProxy: %w", err)
	}

	// Store route
	mf.Routes[route.ID] = route

	logger.Infof("Route %s added to frontend %s: %s%s -> %s", route.ID, frontendID, route.Host, route.Path, route.BackendName)
	return nil
}

// addRouteToHAProxy configures routing in HAProxy
func (fm *FrontendManager) addRouteToHAProxy(frontendName string, route Route) error {
	// Build ACLs and condition based on host and path
	var condTest string
	var aclsToAdd []*models.ACL

	if route.Host != "" && route.Path != "" {
		// Need both host and path ACLs
		hostACLName := fmt.Sprintf("host_%s", sanitizeName(route.Host))
		pathACLName := fmt.Sprintf("path_%s", sanitizeName(route.Path))

		aclsToAdd = append(aclsToAdd, &models.ACL{
			ACLName:   hostACLName,
			Criterion: "hdr(host)",
			Value:     fmt.Sprintf("-i %s", route.Host),
		})
		aclsToAdd = append(aclsToAdd, &models.ACL{
			ACLName:   pathACLName,
			Criterion: "path_beg",
			Value:     route.Path,
		})

		condTest = fmt.Sprintf("%s %s", hostACLName, pathACLName)
	} else if route.Host != "" {
		aclName := fmt.Sprintf("host_%s", sanitizeName(route.Host))
		aclsToAdd = append(aclsToAdd, &models.ACL{
			ACLName:   aclName,
			Criterion: "hdr(host)",
			Value:     fmt.Sprintf("-i %s", route.Host),
		})
		condTest = aclName
	} else if route.Path != "" {
		aclName := fmt.Sprintf("path_%s", sanitizeName(route.Path))
		aclsToAdd = append(aclsToAdd, &models.ACL{
			ACLName:   aclName,
			Criterion: "path_beg",
			Value:     route.Path,
		})
		condTest = aclName
	} else {
		return fmt.Errorf("either host or path must be specified")
	}

	// Start transaction
	if err := fm.haproxyClient.APIStartTransaction(); err != nil {
		return err
	}
	defer fm.haproxyClient.APIDisposeTransaction()

	// Get existing ACLs from the frontend
	existingACLs, err := fm.haproxyClient.ACLsGet("frontend", frontendName)
	if err != nil {
		logger.Debugf("Could not fetch existing ACLs (may not exist yet): %v", err)
		existingACLs = models.Acls{}
	}
	logger.Infof("existingACLs: %v", existingACLs)

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

	// Replace all ACLs
	if err := fm.haproxyClient.ACLsReplace("frontend", frontendName, allACLs); err != nil {
		return fmt.Errorf("failed to update ACLs: %w", err)
	}
	logger.Infof("allACLs: %v", allACLs)
	// Get existing backend switching rules to find the next index
	existingRules, err := fm.haproxyClient.BackendSwitchingRulesGet(frontendName)
	if err != nil {
		logger.Debugf("Could not fetch existing rules (may not exist yet): %v", err)
		existingRules = models.BackendSwitchingRules{}
	}
	nextIndex := int64(len(existingRules))

	// Create backend switching rule
	rule := models.BackendSwitchingRule{
		Cond:     "if",
		CondTest: condTest,
		Name:     route.BackendName,
	}

	if err := fm.haproxyClient.BackendSwitchingRuleCreate(nextIndex, frontendName, rule); err != nil {
		return fmt.Errorf("failed to create backend switching rule: %w", err)
	}
	logger.Infof("Create Rule: %v", rule)
	// Commit transaction
	if err := fm.haproxyClient.APIFinalCommitTransaction(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeleteRoute removes a routing rule from a specific frontend
func (fm *FrontendManager) DeleteRoute(frontendID string, routeID string) error {
	fm.mu.RLock()
	mf, exists := fm.frontends[frontendID]
	fm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("frontend %s not found", frontendID)
	}

	mf.mu.Lock()
	defer mf.mu.Unlock()

	if _, exists := mf.Routes[routeID]; !exists {
		return fmt.Errorf("route %s not found in frontend %s", routeID, frontendID)
	}

	delete(mf.Routes, routeID)
	logger.Infof("Route %s deleted from frontend %s", routeID, frontendID)

	// Note: Actual HAProxy route deletion would require rebuilding all routes
	// This is a simplified implementation
	return nil
}

// GetFrontend returns a managed frontend by ID
func (fm *FrontendManager) GetFrontend(frontendID string) (*ManagedFrontend, error) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	mf, exists := fm.frontends[frontendID]
	if !exists {
		return nil, fmt.Errorf("frontend %s not found", frontendID)
	}

	return mf, nil
}

// ListFrontends returns all managed frontends
func (fm *FrontendManager) ListFrontends() map[string]*ManagedFrontend {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	result := make(map[string]*ManagedFrontend, len(fm.frontends))
	for k, v := range fm.frontends {
		result[k] = v
	}
	return result
}

// GetBackends returns all backends for a specific frontend
func (fm *FrontendManager) GetBackends(frontendID string) (map[string]*Backend, error) {
	fm.mu.RLock()
	mf, exists := fm.frontends[frontendID]
	fm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("frontend %s not found", frontendID)
	}

	return mf.Manager.GetBackends(), nil
}

// GetRoutes returns all routes for a specific frontend
func (fm *FrontendManager) GetRoutes(frontendID string) (map[string]Route, error) {
	fm.mu.RLock()
	mf, exists := fm.frontends[frontendID]
	fm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("frontend %s not found", frontendID)
	}

	mf.mu.RLock()
	defer mf.mu.RUnlock()

	result := make(map[string]Route, len(mf.Routes))
	for k, v := range mf.Routes {
		result[k] = v
	}
	return result, nil
}

// Stop stops all frontends
func (fm *FrontendManager) Stop() error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	logger.Info("Stopping Frontend Manager")

	for id, mf := range fm.frontends {
		if err := mf.Manager.Stop(); err != nil {
			logger.Errorf("Error stopping frontend %s: %v", id, err)
		}
	}

	logger.Info("Frontend Manager stopped")
	return nil
}

// RoutesCount returns how many routes are configured on the frontend. Reading
// len(mf.Routes) directly races AddRoute, which writes the map under mf.mu.
func (mf *ManagedFrontend) RoutesCount() int {
	mf.mu.RLock()
	defer mf.mu.RUnlock()
	return len(mf.Routes)
}

// GetFrontendStats returns statistics for a frontend
func (mf *ManagedFrontend) GetStats() map[string]interface{} {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	backends := mf.Manager.GetBackends()
	return map[string]interface{}{
		"id":              mf.Definition.ID,
		"name":            mf.Definition.Name,
		"mode":            mf.Definition.Mode,
		"enabled":         mf.Definition.Enabled,
		"bindings_count":  len(mf.Definition.Bindings),
		"routes_count":    len(mf.Routes),
		"backends_count":  len(backends),
		"bypass_rules":    mf.Definition.Routing.BypassRules,
		"default_backend": mf.Definition.Routing.DefaultBackend,
	}
}
