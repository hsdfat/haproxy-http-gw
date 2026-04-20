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
	"github.com/haproxytech/kubernetes-ingress/pkg/logger"
	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
	"github.com/jessevdk/go-flags"
)

func main() {
	log := logger.New("http-gw", "info")

	// Parse command-line flags
	var osArgs utils.OSArgs
	parser := flags.NewParser(&osArgs, flags.Default)
	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		log.Error(err)
		os.Exit(1)
	}

	// Get runtime socket from environment or use default
	runtimeSocket := os.Getenv("HAPROXY_RUNTIME_SOCKET")
	if runtimeSocket == "" {
		runtimeSocket = "/var/run/haproxy-runtime-api.sock"
	}
	log.Infow("Using HAProxy runtime socket", "socket", runtimeSocket)

	// Initialize HAProxy API client
	haproxyClient, err := api.New(
		"/tmp/haproxy-gateway",          // transaction dir
		"/etc/haproxy/haproxy.cfg",      // config file
		"/usr/local/sbin/haproxy",       // haproxy binary
		runtimeSocket,                    // runtime socket
	)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}

	// Check if frontend config file is specified
	if osArgs.FrontendConfigFile != "" {
		log.Infow("Starting with frontend management mode", "config", osArgs.FrontendConfigFile)
		runFrontendManagementMode(haproxyClient, osArgs)
	} else {
		log.Infow("Starting in legacy mode (single frontend)")
		runSimpleProviderExample(haproxyClient)
	}
}

// monitorHAProxyReload monitors the reload flag and triggers HAProxy reload when needed
func monitorHAProxyReload(ctx context.Context, log logger.Logger) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if instance.NeedReload() {
				log.Infow("Reloading HAProxy due to backend update...")
				if err := reloadHAProxy(log); err != nil {
					log.Errorw("Failed to reload HAProxy", "error", err)
				} else {
					log.Infow("HAProxy reloaded successfully")
					instance.Reset()
				}
			}
		}
	}
}

// reloadHAProxy performs a graceful reload of HAProxy using SIGUSR2 signal
func reloadHAProxy(log logger.Logger) error {
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
		log.Errorw("HAProxy config validation failed", "output", string(output))
		return err
	}

	log.Debugw("HAProxy configuration validated successfully")

	// Get the PID of the current HAProxy master process
	pidFile := os.Getenv("HAPROXY_PID_FILE")
	if pidFile == "" {
		pidFile = "/tmp/haproxy-gateway/haproxy.pid"
	}

	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		log.Warnw("Could not read PID file", "pidFile", pidFile, "error", err)
		return err
	}

	// Parse PID and trim whitespace
	pidStr := string(pidData)
	pidStr = string([]rune(pidStr)[:len(pidStr)-1])

	log.Debugw("Sending SIGUSR2 to HAProxy master process for reload", "pid", pidStr)

	// Send SIGUSR2 to HAProxy master process for graceful reload
	// In master-worker mode, SIGUSR2 triggers a reload
	killCmd := exec.Command("kill", "-USR2", pidStr)
	if output, err := killCmd.CombinedOutput(); err != nil {
		log.Errorw("Failed to send reload signal to HAProxy", "output", string(output))
		return err
	}

	log.Debugw("Reload signal sent successfully to HAProxy master process")
	return nil
}

// Example 1: Simple Provider
func runSimpleProviderExample(haproxyClient api.HAProxyClient) {
	log := logger.New("http-gw", "info")
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
		log.Error(err)
		os.Exit(1)
	}

	// Start API server for dynamic configuration
	apiServer := gateway.NewAPIServer(gw, 9091)
	go func() {
		if err := apiServer.Start(); err != nil && err != http.ErrServerClosed {
			log.Errorw("API server error", "error", err)
		}
	}()

	// Start HAProxy reload monitor
	go monitorHAProxyReload(ctx, log)

	log.Infow("Gateway is running and ready to accept configuration")
	log.Infow("API server listening on :9090")
	log.Infow("")
	log.Infow("Backend Registration API:")
	log.Infow("  POST /api/backends - Register a backend")
	log.Infow("  GET  /api/backends - List all backends")
	log.Infow("  DELETE /api/backends/{name} - Unregister a backend")
	log.Infow("")
	log.Infow("Example backend registration:")
	log.Infow("  curl -X POST http://localhost:9090/api/backends -H 'Content-Type: application/json' -d '{\"name\":\"api-backend\",\"servers\":[{\"name\":\"server1\",\"ip\":\"192.168.1.10\",\"port\":9000}]}'")
	log.Infow("")
	log.Infow("Route Configuration API:")
	log.Infow("  POST /api/routes - Add a routing rule")
	log.Infow("  GET  /api/routes - List all routes")
	log.Infow("Example: curl -X POST http://localhost:9090/api/routes -d '{\"host\":\"api.example.com\",\"path\":\"/api\",\"backend\":\"api-backend\"}'")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Infow("Shutting down gateway...")
	apiServer.Stop()
	cancel()
	gw.Stop()
}

// Example 2: Polling Provider
func runPollingProviderExample(haproxyClient api.HAProxyClient) {
	log := logger.New("http-gw", "info")
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
		log.Error(err)
		os.Exit(1)
	}

	log.Infow("Dynamic gateway is running on port 9080")
	log.Infow("Provider will poll for backend updates every 10 seconds")

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Infow("Shutting down...")
	cancel()
	gw.Stop()
}

// runFrontendManagementMode runs the gateway in frontend management mode
// with support for multiple frontends configured via YAML or flags
func runFrontendManagementMode(haproxyClient api.HAProxyClient, osArgs utils.OSArgs) {
	log := logger.New("http-gw", "info")
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
		log.Errorw("Failed to load frontend configuration", "error", err)
		os.Exit(1)
	}

	log.Infow("Loaded configuration with frontends", "count", len(config.Frontends))
	for _, fe := range config.Frontends {
		log.Infow("Frontend loaded", "name", fe.Name, "id", fe.ID, "bindings", len(fe.Bindings))
	}

	// Create FrontendManager
	frontendManager := gateway.NewFrontendManager(haproxyClient, *config)

	// Start all frontends
	if err := frontendManager.Start(ctx); err != nil {
		log.Errorw("Failed to start frontends", "error", err)
		os.Exit(1)
	}

	// Start Enhanced API Server (supports frontend-scoped operations)
	apiPort := 9090
	enhancedAPI := gateway.NewEnhancedAPIServer(frontendManager, apiPort)
	go func() {
		if err := enhancedAPI.Start(); err != nil && err != http.ErrServerClosed {
			log.Errorw("Enhanced API server error", "error", err)
		}
	}()

	// Start HAProxy reload monitor
	go monitorHAProxyReload(ctx, log)

	log.Infow("Frontend Management Gateway is running")
	log.Infow("Enhanced API server listening", "port", apiPort)
	log.Infow("")
	log.Infow("Frontend Management API:")
	log.Infow("  GET    /api/frontends - List all frontends")
	log.Infow("  GET    /api/frontends/{id} - Get frontend details")
	log.Infow("  GET    /api/frontends/{id}/stats - Get frontend statistics")
	log.Infow("")
	log.Infow("Backend Management API (per frontend):")
	log.Infow("  POST   /api/frontends/{id}/backends - Register backend to frontend")
	log.Infow("  GET    /api/frontends/{id}/backends - List backends for frontend")
	log.Infow("  DELETE /api/frontends/{id}/backends/{name} - Unregister backend")
	log.Infow("")
	log.Infow("Route Management API (per frontend):")
	log.Infow("  POST   /api/frontends/{id}/routes - Add route to frontend")
	log.Infow("  GET    /api/frontends/{id}/routes - List routes for frontend")
	log.Infow("  DELETE /api/frontends/{id}/routes/{route_id} - Delete route")
	log.Infow("")
	log.Infow("Per-Path Overload Protection API (per frontend, requires overload_enabled):")
	log.Infow("  POST   /api/frontends/{id}/overload - Upsert path limit ({path, limit})")
	log.Infow("  GET    /api/frontends/{id}/overload - List rules")
	log.Infow("  DELETE /api/frontends/{id}/overload?path=/p - Delete rule")
	log.Infow("  GET    /api/frontends/{id}/overload/stats - Show current rates")
	log.Infow("")
	log.Infow("Example usage:")
	log.Infow("  # List frontends")
	log.Infow("  curl http://localhost:9090/api/frontends")
	log.Infow("")
	log.Infow("  # Register backend to specific frontend")
	log.Infow("  curl -X POST http://localhost:9090/api/frontends/public-api/backends \\")
	log.Infow("    -H 'Content-Type: application/json' \\")
	log.Infow("    -d '{\"name\":\"api-backend\",\"servers\":[{\"name\":\"srv1\",\"ip\":\"10.0.1.10\",\"port\":8080}]}'")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Infow("Shutting down frontend management gateway...")
	enhancedAPI.Stop()
	cancel()
	frontendManager.Stop()
}
