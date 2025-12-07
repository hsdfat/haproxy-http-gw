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
	"os/signal"
	"syscall"
	"time"

	"github.com/haproxytech/kubernetes-ingress/pkg/gateway"
	"github.com/haproxytech/kubernetes-ingress/pkg/gateway/examples"
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

func main() {
	logger := utils.GetLogger()
	logger.SetLevel(utils.Info)

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

	// Example 1: Simple Provider with manual backend management
	logger.Info("=== Example 1: Simple Provider ===")
	runSimpleProviderExample(haproxyClient)
}

// Example 1: Simple Provider
func runSimpleProviderExample(haproxyClient api.HAProxyClient) {
	logger := utils.GetLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a simple provider
	provider := examples.NewSimpleProvider()

	// Add some backends manually (using actual test environment servers)
	provider.AddBackend(gateway.Backend{
		Name: "api-backend",
		Servers: []gateway.BackendServer{
			{Name: "backend-server-1", IP: "backend-server-1", Port: 9000},
			{Name: "backend-server-2", IP: "backend-server-2", Port: 9000},
			{Name: "backend-server-3", IP: "backend-server-3", Port: 9000},
		},
	})

	provider.AddBackend(gateway.Backend{
		Name: "web-backend",
		Servers: []gateway.BackendServer{
			{Name: "web-server-1", IP: "web-server-1", Port: 9000},
			{Name: "web-server-2", IP: "web-server-2", Port: 9000},
		},
	})

	// Create backend manager
	manager := gateway.NewManager(gateway.ManagerConfig{
		HAProxyClient: haproxyClient,
		Provider:      provider,
		SyncPeriod:    5 * time.Second,
		EventChanSize: 100,
	})

	// Create HTTP gateway with HTTP/2 support
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

	// Start the gateway
	if err := gw.Start(ctx); err != nil {
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

	logger.Info("Gateway is running and ready to accept configuration")
	logger.Info("API server listening on :9090")
	logger.Info("Use POST /api/routes to add routing rules")
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
	manager := gateway.NewManager(gateway.ManagerConfig{
		HAProxyClient: haproxyClient,
		Provider:      provider,
		SyncPeriod:    5 * time.Second,
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
