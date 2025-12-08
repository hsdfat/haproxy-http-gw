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

package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/haproxytech/kubernetes-ingress/pkg/gateway"
	"github.com/haproxytech/kubernetes-ingress/pkg/gateway/examples"
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/instance"
	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
	"github.com/jessevdk/go-flags"
)

func main() {
	logger := utils.GetLogger()
	logger.SetLevel(utils.Info)

	// Parse command-line flags
	var osArgs utils.OSArgs
	parser := flags.NewParser(&osArgs, flags.Default)
	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		logger.Error(err)
		os.Exit(1)
	}

	// Get runtime socket from environment or use default
	runtimeSocket := os.Getenv("HAPROXY_RUNTIME_SOCKET")
	if runtimeSocket == "" {
		runtimeSocket = "/var/run/haproxy-runtime-api.sock"
	}
	logger.Infof("Using HAProxy runtime socket: %s", runtimeSocket)

	// Initialize HAProxy API client
	haproxyClient, err := api.New(
		"/tmp/haproxy-gateway",          // transaction dir
		"/etc/haproxy/haproxy.cfg",      // config file
		"/usr/local/sbin/haproxy",       // haproxy binary
		runtimeSocket,                    // runtime socket
	)
	if err != nil {
		logger.Error(err)
		os.Exit(1)
	}

	// Check if frontend config file is specified
	if osArgs.FrontendConfigFile != "" {
		logger.Infof("Starting with frontend management mode (config: %s)", osArgs.FrontendConfigFile)
		runFrontendManagementMode(haproxyClient, osArgs)
	} else {
		logger.Info("Starting in legacy mode (single frontend)")
		runSimpleProviderExample(haproxyClient)
	}
}

// monitorHAProxyReload monitors the reload flag and triggers HAProxy reload when needed
func monitorHAProxyReload(ctx context.Context, logger utils.Logger) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if instance.NeedReload() {
				logger.Info("Reloading HAProxy due to backend update...")
				if err := reloadHAProxy(logger); err != nil {
					logger.Errorf("Failed to reload HAProxy: %v", err)
				} else {
					logger.Info("HAProxy reloaded successfully")
					instance.Reset()
				}
			}
		}
	}
}

// reloadHAProxy performs a graceful reload of HAProxy using SIGUSR2 signal
func reloadHAProxy(logger utils.Logger) error {
	// Get HAProxy binary path from environment or use default
	haproxyBin := os.Getenv("HAPROXY_BIN")
	if haproxyBin == "" {
		haproxyBin = "/usr/local/sbin/haproxy"
	}

	configFile := os.Getenv("HAPROXY_CONFIG")
	if configFile == "" {
		configFile = "/etc/haproxy/haproxy.cfg"
	}

	// First, validate the configuration
	validateCmd := exec.Command(haproxyBin, "-c", "-f", configFile)
	if output, err := validateCmd.CombinedOutput(); err != nil {
		logger.Errorf("HAProxy config validation failed: %s", string(output))
		return err
	}

	logger.Debug("HAProxy configuration validated successfully")

	// Get the PID of the current HAProxy master process
	pidFile := os.Getenv("HAPROXY_PID_FILE")
	if pidFile == "" {
		pidFile = "/tmp/haproxy-gateway/haproxy.pid"
	}

	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		logger.Warningf("Could not read PID file %s: %v", pidFile, err)
		return err
	}

	// Parse PID and trim whitespace
	pidStr := string(pidData)
	pidStr = string([]rune(pidStr)[:len(pidStr)-1])

	logger.Debugf("Sending SIGUSR2 to HAProxy master process (PID: %s) for reload", pidStr)

	// Send SIGUSR2 to HAProxy master process for graceful reload
	// In master-worker mode, SIGUSR2 triggers a reload
	killCmd := exec.Command("kill", "-USR2", pidStr)
	if output, err := killCmd.CombinedOutput(); err != nil {
		logger.Errorf("Failed to send reload signal to HAProxy: %s", string(output))
		return err
	}

	logger.Debug("Reload signal sent successfully to HAProxy master process")
	return nil
}

// Example 1: Simple Provider
func runSimpleProviderExample(haproxyClient api.HAProxyClient) {
	logger := utils.GetLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a simple provider (backends will be registered via API)
	provider := examples.NewSimpleProvider()

	// Create backend manager
	// Note: SyncPeriod is set to 5 minutes to avoid frequent reloads
	// that would wipe out dynamically added routes
	manager := gateway.NewManager(gateway.ManagerConfig{
		HAProxyClient: haproxyClient,
		Provider:      provider,
		SyncPeriod:    5 * time.Minute, // Reduced frequency to preserve routes
		EventChanSize: 100,
	})

	// Create HTTP gateway with HTTP/2 support
	// Frontend is statically configured in haproxy-init.cfg
	// For test environment: HTTPS disabled to avoid cert validation errors
	gw := gateway.NewHTTPGateway(haproxyClient, manager, gateway.GatewayConfig{
		FrontendName: "http-gateway",
		HTTPPort:     8080,
		HTTPSPort:    8443,
		HTTPSEnabled: false,  // Disabled for test environment
		SSLCertDir:   "/etc/haproxy/certs",
		EnableHTTP2:  true,
		ALPN:         "h2,http/1.1",
		DefaultBackend: "api-backend",
	})

	// Start the gateway (backend manager only, frontend is pre-configured)
	if err := gw.StartWithoutFrontend(ctx); err != nil {
			logger.Error(err)
		os.Exit(1)
	}

	// Start API server for dynamic configuration
	apiServer := gateway.NewAPIServer(gw, 9090)
	go func() {
		if err := apiServer.Start(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("API server error: %v", err)
		}
	}()

	// Start HAProxy reload monitor
	go monitorHAProxyReload(ctx, logger)

	logger.Info("Gateway is running and ready to accept configuration")
	logger.Info("API server listening on :9090")
	logger.Info("")
	logger.Info("Backend Registration API:")
	logger.Info("  POST /api/backends - Register a backend")
	logger.Info("  GET  /api/backends - List all backends")
	logger.Info("  DELETE /api/backends/{name} - Unregister a backend")
	logger.Info("")
	logger.Info("Example backend registration:")
	logger.Info("  curl -X POST http://localhost:9090/api/backends -H 'Content-Type: application/json' -d '{\"name\":\"api-backend\",\"servers\":[{\"name\":\"server1\",\"ip\":\"192.168.1.10\",\"port\":9000}]}'")
	logger.Info("")
	logger.Info("Route Configuration API:")
	logger.Info("  POST /api/routes - Add a routing rule")
	logger.Info("  GET  /api/routes - List all routes")
	logger.Info("Example: curl -X POST http://localhost:9090/api/routes -d '{\"host\":\"api.example.com\",\"path\":\"/api\",\"backend\":\"api-backend\"}'")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down gateway...")
	apiServer.Stop()
	cancel()
	gw.Stop()
}

// Example 2: Polling Provider
func runPollingProviderExample(haproxyClient api.HAProxyClient) {
	logger := utils.GetLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a polling provider that fetches backends from a function
	// In production, this would fetch from a database, REST API, service registry, etc.
	provider := examples.NewPollingProvider(10*time.Second, func() ([]gateway.Backend, error) {
		// This function would query your backend source
		// For demo purposes, we return static data
		return []gateway.Backend{
			{
				Name: "dynamic-backend-1",
				Servers: []gateway.BackendServer{
					{Name: "srv1", IP: "192.168.1.10", Port: 9000},
					{Name: "srv2", IP: "192.168.1.11", Port: 9000},
				},
			},
			{
				Name: "dynamic-backend-2",
				Servers: []gateway.BackendServer{
					{Name: "srv1", IP: "192.168.2.10", Port: 9000},
				},
			},
		}, nil
	})

	// Create backend manager
	// Note: SyncPeriod is set to 5 minutes to avoid frequent reloads
	// that would wipe out dynamically added routes
	manager := gateway.NewManager(gateway.ManagerConfig{
		HAProxyClient: haproxyClient,
		Provider:      provider,
		SyncPeriod:    5 * time.Minute, // Reduced frequency to preserve routes
		EventChanSize: 100,
	})

	// Create HTTP gateway
	gw := gateway.NewHTTPGateway(haproxyClient, manager, gateway.GatewayConfig{
		FrontendName: "dynamic-gateway",
		HTTPPort:     9080,
		HTTPSPort:    9443,
		HTTPSEnabled: false, // Disable HTTPS for this example
		EnableHTTP2:  true,
		ALPN:         "h2,http/1.1",
		DefaultBackend: "dynamic-backend-1",
	})

	// Start the gateway
	if err := gw.Start(ctx); err != nil {
			logger.Error(err)
		os.Exit(1)
	}

	logger.Info("Dynamic gateway is running on port 9080")
	logger.Info("Provider will poll for backend updates every 10 seconds")

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down...")
	cancel()
	gw.Stop()
}

// runFrontendManagementMode runs the gateway in frontend management mode
// with support for multiple frontends configured via YAML or flags
func runFrontendManagementMode(haproxyClient api.HAProxyClient, osArgs utils.OSArgs) {
	logger := utils.GetLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create configuration registry with providers in priority order
	registry := gateway.NewConfigRegistry()

	// 1. Try YAML config file first (highest priority)
	if osArgs.FrontendConfigFile != "" {
		registry.Register(gateway.NewYAMLConfigProvider(osArgs.FrontendConfigFile))
	}

	// 2. Fall back to command-line flags (backward compatibility)
	registry.Register(gateway.NewFlagConfigProvider(osArgs))

	// Load configuration
	config, err := registry.LoadConfig()
	if err != nil {
		logger.Errorf("Failed to load frontend configuration: %v", err)
		os.Exit(1)
	}

	logger.Infof("Loaded configuration with %d frontend(s)", len(config.Frontends))
	for _, fe := range config.Frontends {
		logger.Infof("  - Frontend '%s' (ID: %s): %d binding(s)", fe.Name, fe.ID, len(fe.Bindings))
	}

	// Create FrontendManager
	frontendManager := gateway.NewFrontendManager(haproxyClient, *config)

	// Start all frontends
	if err := frontendManager.Start(ctx); err != nil {
		logger.Errorf("Failed to start frontends: %v", err)
		os.Exit(1)
	}

	// Start Enhanced API Server (supports frontend-scoped operations)
	apiPort := 9090
	enhancedAPI := gateway.NewEnhancedAPIServer(frontendManager, apiPort)
	go func() {
		if err := enhancedAPI.Start(); err != nil && err != http.ErrServerClosed {
			logger.Errorf("Enhanced API server error: %v", err)
		}
	}()

	// Start HAProxy reload monitor
	go monitorHAProxyReload(ctx, logger)

	logger.Info("Frontend Management Gateway is running")
	logger.Infof("Enhanced API server listening on :%d", apiPort)
	logger.Info("")
	logger.Info("Frontend Management API:")
	logger.Info("  GET    /api/frontends - List all frontends")
	logger.Info("  GET    /api/frontends/{id} - Get frontend details")
	logger.Info("  GET    /api/frontends/{id}/stats - Get frontend statistics")
	logger.Info("")
	logger.Info("Backend Management API (per frontend):")
	logger.Info("  POST   /api/frontends/{id}/backends - Register backend to frontend")
	logger.Info("  GET    /api/frontends/{id}/backends - List backends for frontend")
	logger.Info("  DELETE /api/frontends/{id}/backends/{name} - Unregister backend")
	logger.Info("")
	logger.Info("Route Management API (per frontend):")
	logger.Info("  POST   /api/frontends/{id}/routes - Add route to frontend")
	logger.Info("  GET    /api/frontends/{id}/routes - List routes for frontend")
	logger.Info("  DELETE /api/frontends/{id}/routes/{route_id} - Delete route")
	logger.Info("")
	logger.Info("Example usage:")
	logger.Info("  # List frontends")
	logger.Info("  curl http://localhost:9090/api/frontends")
	logger.Info("")
	logger.Info("  # Register backend to specific frontend")
	logger.Info("  curl -X POST http://localhost:9090/api/frontends/public-api/backends \\")
	logger.Info("    -H 'Content-Type: application/json' \\")
	logger.Info("    -d '{\"name\":\"api-backend\",\"servers\":[{\"name\":\"srv1\",\"ip\":\"10.0.1.10\",\"port\":8080}]}'")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down frontend management gateway...")
	enhancedAPI.Stop()
	cancel()
	frontendManager.Stop()
}
