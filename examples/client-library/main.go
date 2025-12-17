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
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/haproxytech/kubernetes-ingress/pkg/client"
	"github.com/haproxytech/kubernetes-ingress/pkg/gateway"
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
)

func main() {
	fmt.Println("HAProxy HTTP Gateway - Client Library Example")
	fmt.Println("==============================================")

	// Create HAProxy client
	haproxyClient, err := createHAProxyClient()
	if err != nil {
		log.Fatalf("Failed to create HAProxy client: %v", err)
	}

	// Create frontend configuration
	config := gateway.FrontendConfig{
		Frontends: []gateway.FrontendDefinition{
			{
				ID:      "default",
				Name:    "http-frontend",
				Enabled: true,
				Bindings: []gateway.BindingDefinition{
					{
						Address: "*",
						Port:    8080,
					},
				},
			},
		},
	}

	// Create frontend manager
	fm := gateway.NewFrontendManager(haproxyClient, config)

	// Start frontend manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := fm.Start(ctx); err != nil {
		log.Fatalf("Failed to start frontend manager: %v", err)
	}

	// Create gateway client
	c := client.NewClient(fm)

	fmt.Println("\n✓ Gateway client initialized")

	// Example 1: Register a single backend
	fmt.Println("\n=== Example 1: Register Single Backend ===")
	singleBackend := gateway.Backend{
		Name: "web-backend",
		Servers: []gateway.BackendServer{
			{
				Name: "web-1",
				IP:   "192.168.1.10",
				Port: 8080,
			},
		},
	}

	if err := c.RegisterBackend("default", singleBackend); err != nil {
		log.Printf("Failed to register backend: %v", err)
	} else {
		fmt.Printf("✓ Registered backend: %s with 1 server\n", singleBackend.Name)
	}

	time.Sleep(2 * time.Second)

	// Example 2: Register a multi-server backend
	fmt.Println("\n=== Example 2: Register Multi-Server Backend ===")
	multiBackend := gateway.Backend{
		Name: "api-backend",
		Servers: []gateway.BackendServer{
			{
				Name: "api-1",
				IP:   "192.168.1.20",
				Port: 9000,
			},
			{
				Name: "api-2",
				IP:   "192.168.1.21",
				Port: 9000,
			},
			{
				Name: "api-3",
				IP:   "192.168.1.22",
				Port: 9000,
			},
		},
	}

	if err := c.RegisterBackend("default", multiBackend); err != nil {
		log.Printf("Failed to register backend: %v", err)
	} else {
		fmt.Printf("✓ Registered backend: %s with %d servers\n", multiBackend.Name, len(multiBackend.Servers))
	}

	time.Sleep(2 * time.Second)

	// Example 3: List all backends
	fmt.Println("\n=== Example 3: List All Backends ===")
	backends, err := c.GetBackends("default")
	if err != nil {
		log.Printf("Failed to get backends: %v", err)
	} else {
		fmt.Printf("Total backends: %d\n", len(backends))
		for name, backend := range backends {
			fmt.Printf("  - %s: %d server(s)\n", name, len(backend.Servers))
			for _, server := range backend.Servers {
				fmt.Printf("    • %s: %s:%d\n", server.Name, server.IP, server.Port)
			}
		}
	}

	// Example 4: Add routing rules
	fmt.Println("\n=== Example 4: Add Routing Rules ===")
	route1 := gateway.Route{
		ID:          "api-route",
		Host:        "api.example.com",
		Path:        "/v1",
		BackendName: "api-backend",
		FrontendID:  "default",
	}

	if err := c.AddRoute("default", route1); err != nil {
		log.Printf("Failed to add route: %v", err)
	} else {
		fmt.Printf("✓ Added route: %s -> %s (host: %s, path: %s)\n",
			route1.ID, route1.BackendName, route1.Host, route1.Path)
	}

	// Example 5: Unregister a backend
	fmt.Println("\n=== Example 5: Unregister Backend ===")
	if err := c.UnregisterBackend("default", "web-backend"); err != nil {
		log.Printf("Failed to unregister backend: %v", err)
	} else {
		fmt.Println("✓ Unregistered backend: web-backend")
	}

	time.Sleep(2 * time.Second)

	// Example 6: Verify backend was removed
	fmt.Println("\n=== Example 6: Verify Backend Removal ===")
	backends, err = c.GetBackends("default")
	if err != nil {
		log.Printf("Failed to get backends: %v", err)
	} else {
		if _, exists := backends["web-backend"]; !exists {
			fmt.Println("✓ Backend 'web-backend' successfully removed")
		} else {
			fmt.Println("✗ Backend 'web-backend' still exists")
		}
		fmt.Printf("Remaining backends: %d\n", len(backends))
	}

	// Example 7: Concurrent backend registration
	fmt.Println("\n=== Example 7: Concurrent Backend Registration ===")
	errChan := make(chan error, 3)

	for i := 1; i <= 3; i++ {
		go func(index int) {
			backend := gateway.Backend{
				Name: fmt.Sprintf("concurrent-backend-%d", index),
				Servers: []gateway.BackendServer{
					{
						Name: "srv1",
						IP:   fmt.Sprintf("10.0.%d.10", index),
						Port: 8080,
					},
				},
			}
			errChan <- c.RegisterBackend("default", backend)
		}(i)
	}

	// Wait for all concurrent registrations
	successCount := 0
	for i := 0; i < 3; i++ {
		if err := <-errChan; err != nil {
			log.Printf("Concurrent registration error: %v", err)
		} else {
			successCount++
		}
	}

	fmt.Printf("✓ Successfully registered %d backends concurrently\n", successCount)

	// Wait for user interrupt
	fmt.Println("\n==============================================")
	fmt.Println("Gateway is running. Press Ctrl+C to stop.")
	fmt.Println("==============================================")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")

	// Cleanup
	if err := fm.Stop(); err != nil {
		log.Printf("Error stopping frontend manager: %v", err)
	}

	fmt.Println("✓ Gateway stopped")
}

// createHAProxyClient creates and configures the HAProxy client
func createHAProxyClient() (api.HAProxyClient, error) {
	// This would normally connect to HAProxy Data Plane API
	// For this example, we'll use a mock or test client
	// In production, you would use:
	//
	// return api.NewClient(api.ClientConfig{
	//     DataPlaneAPI: "http://localhost:5555/v2",
	//     Username:     "admin",
	//     Password:     "adminpwd",
	// })

	// For now, return an error indicating this needs to be configured
	return nil, fmt.Errorf("HAProxy client creation not implemented - configure Data Plane API connection")
}
