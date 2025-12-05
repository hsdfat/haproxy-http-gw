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

	// Initialize HAProxy API client
	haproxyClient, err := api.New(
		"/tmp/haproxy-gateway",          // transaction dir
		"/etc/haproxy/haproxy.cfg",      // config file
		"/usr/local/sbin/haproxy",       // haproxy binary
		"/var/run/haproxy-runtime-api.sock", // runtime socket
	)
	if err != nil {
		logger.Error(err)
		os.Exit(1)
	}

	// Example 1: Simple Provider with manual backend management
	logger.Info("=== Example 1: Simple Provider ===")
	runSimpleProviderExample(haproxyClient)

	// Example 2: Polling Provider with dynamic backend discovery
	logger.Info("\n=== Example 2: Polling Provider ===")
	runPollingProviderExample(haproxyClient)
}

// Example 1: Simple Provider
func runSimpleProviderExample(haproxyClient api.HAProxyClient) {
	logger := utils.GetLogger()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a simple provider
	provider := examples.NewSimpleProvider()

	// Add some backends manually
	provider.AddBackend(gateway.Backend{
		Name: "api-backend",
		Servers: []gateway.BackendServer{
			{Name: "api-server-1", IP: "10.0.1.10", Port: 8080},
			{Name: "api-server-2", IP: "10.0.1.11", Port: 8080},
		},
	})

	provider.AddBackend(gateway.Backend{
		Name: "web-backend",
		Servers: []gateway.BackendServer{
			{Name: "web-server-1", IP: "10.0.2.10", Port: 80},
			{Name: "web-server-2", IP: "10.0.2.11", Port: 80},
			{Name: "web-server-3", IP: "10.0.2.12", Port: 80},
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
	gw := gateway.NewHTTPGateway(haproxyClient, manager, gateway.GatewayConfig{
		FrontendName: "http-gateway",
		HTTPPort:     8080,
		HTTPSPort:    8443,
		HTTPSEnabled: true,
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

	// Add routing rules
	if err := gw.AddBackendRoute("api.example.com", "/api", "api-backend"); err != nil {
		logger.Errorf("Failed to add API route: %v", err)
	}

	if err := gw.AddBackendRoute("www.example.com", "/", "web-backend"); err != nil {
		logger.Errorf("Failed to add web route: %v", err)
	}

	logger.Info("Gateway is running. Routes configured:")
	logger.Info("  - api.example.com/api -> api-backend")
	logger.Info("  - www.example.com/    -> web-backend")

	// Simulate a backend update after 5 seconds
	time.Sleep(5 * time.Second)
	logger.Info("Updating api-backend...")
	provider.UpdateBackend(gateway.Backend{
		Name: "api-backend",
		Servers: []gateway.BackendServer{
			{Name: "api-server-1", IP: "10.0.1.10", Port: 8080},
			{Name: "api-server-2", IP: "10.0.1.11", Port: 8080},
			{Name: "api-server-3", IP: "10.0.1.12", Port: 8080}, // New server
		},
	})

	// Wait for shutdown
	<-ctx.Done()
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
