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

package client_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/haproxytech/kubernetes-ingress/pkg/client"
	"github.com/haproxytech/kubernetes-ingress/pkg/gateway"
)

// TestDynamicBackendRegistration tests backend registration/deregistration using the client library
func TestDynamicBackendRegistration(t *testing.T) {
	// This test requires a running HAProxy instance
	// Skip if not in integration test mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup: Get the gateway's frontend manager
	// In a real scenario, this would be obtained from the running gateway instance
	fm := getTestFrontendManager(t)
	if fm == nil {
		t.Skip("Frontend manager not available")
	}

	// Create client
	c := client.NewClient(fm)

	// Test cases
	tests := []struct {
		name        string
		frontendID  string
		backend     gateway.Backend
		shouldError bool
	}{
		{
			name:       "Register single server backend",
			frontendID: "default",
			backend: gateway.Backend{
				Name: "test-backend-1",
				Servers: []gateway.BackendServer{
					{
						Name: "srv1",
						IP:   "127.0.0.1",
						Port: 8080,
					},
				},
			},
			shouldError: false,
		},
		{
			name:       "Register multiple server backend",
			frontendID: "default",
			backend: gateway.Backend{
				Name: "test-backend-2",
				Servers: []gateway.BackendServer{
					{
						Name: "srv1",
						IP:   "127.0.0.1",
						Port: 8080,
					},
					{
						Name: "srv2",
						IP:   "127.0.0.1",
						Port: 8081,
					},
					{
						Name: "srv3",
						IP:   "127.0.0.1",
						Port: 8082,
					},
				},
			},
			shouldError: false,
		},
		{
			name:       "Register to non-existent frontend",
			frontendID: "non-existent",
			backend: gateway.Backend{
				Name: "test-backend-3",
				Servers: []gateway.BackendServer{
					{
						Name: "srv1",
						IP:   "127.0.0.1",
						Port: 9000,
					},
				},
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Register backend
			err := c.RegisterBackend(tt.frontendID, tt.backend)
			if (err != nil) != tt.shouldError {
				t.Errorf("RegisterBackend() error = %v, shouldError %v", err, tt.shouldError)
				return
			}

			if tt.shouldError {
				return // Expected error, test passed
			}

			// Wait for registration to complete
			time.Sleep(2 * time.Second)

			// Verify backend is registered
			backends, err := c.GetBackends(tt.frontendID)
			if err != nil {
				t.Fatalf("GetBackends() error = %v", err)
			}

			backend, exists := backends[tt.backend.Name]
			if !exists {
				t.Errorf("Backend %s not found after registration", tt.backend.Name)
				return
			}

			// Verify server count
			if len(backend.Servers) != len(tt.backend.Servers) {
				t.Errorf("Expected %d servers, got %d", len(tt.backend.Servers), len(backend.Servers))
			}

			// Unregister backend
			err = c.UnregisterBackend(tt.frontendID, tt.backend.Name)
			if err != nil {
				t.Errorf("UnregisterBackend() error = %v", err)
			}

			// Wait for unregistration to complete
			time.Sleep(2 * time.Second)

			// Verify backend is removed
			backends, err = c.GetBackends(tt.frontendID)
			if err != nil {
				t.Fatalf("GetBackends() error = %v", err)
			}

			if _, exists := backends[tt.backend.Name]; exists {
				t.Errorf("Backend %s still exists after unregistration", tt.backend.Name)
			}
		})
	}
}

// TestRapidRegistrationDeregistration tests rapid cycles of backend registration/deregistration
func TestRapidRegistrationDeregistration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	fm := getTestFrontendManager(t)
	if fm == nil {
		t.Skip("Frontend manager not available")
	}

	c := client.NewClient(fm)
	frontendID := "default"

	// Perform 10 rapid register/deregister cycles
	for i := 1; i <= 10; i++ {
		backendName := "rapid-backend"
		backend := gateway.Backend{
			Name: backendName,
			Servers: []gateway.BackendServer{
				{
					Name: "srv1",
					IP:   "127.0.0.1",
					Port: 8080 + i,
				},
			},
		}

		// Register
		if err := c.RegisterBackend(frontendID, backend); err != nil {
			t.Fatalf("Cycle %d: RegisterBackend() error = %v", i, err)
		}

		time.Sleep(500 * time.Millisecond)

		// Verify
		backends, err := c.GetBackends(frontendID)
		if err != nil {
			t.Fatalf("Cycle %d: GetBackends() error = %v", i, err)
		}

		if _, exists := backends[backendName]; !exists {
			t.Errorf("Cycle %d: Backend not found after registration", i)
		}

		// Unregister
		if err := c.UnregisterBackend(frontendID, backendName); err != nil {
			t.Fatalf("Cycle %d: UnregisterBackend() error = %v", i, err)
		}

		time.Sleep(500 * time.Millisecond)

		// Verify removal
		backends, err = c.GetBackends(frontendID)
		if err != nil {
			t.Fatalf("Cycle %d: GetBackends() error = %v", i, err)
		}

		if _, exists := backends[backendName]; exists {
			t.Errorf("Cycle %d: Backend still exists after unregistration", i)
		}
	}

	t.Logf("Successfully completed 10 rapid register/deregister cycles")
}

// TestConcurrentBackendOperations tests concurrent backend registration
func TestConcurrentBackendOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	fm := getTestFrontendManager(t)
	if fm == nil {
		t.Skip("Frontend manager not available")
	}

	c := client.NewClient(fm)
	frontendID := "default"

	// Register 5 backends concurrently
	errChan := make(chan error, 5)
	for i := 1; i <= 5; i++ {
		go func(index int) {
			backend := gateway.Backend{
				Name: fmt.Sprintf("concurrent-backend-%d", index),
				Servers: []gateway.BackendServer{
					{
						Name: "srv1",
						IP:   "127.0.0.1",
						Port: 9000 + index,
					},
				},
			}
			errChan <- c.RegisterBackend(frontendID, backend)
		}(i)
	}

	// Wait for all registrations
	for i := 0; i < 5; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("Concurrent registration error: %v", err)
		}
	}

	time.Sleep(2 * time.Second)

	// Verify all backends are registered
	backends, err := c.GetBackends(frontendID)
	if err != nil {
		t.Fatalf("GetBackends() error = %v", err)
	}

	for i := 1; i <= 5; i++ {
		backendName := fmt.Sprintf("concurrent-backend-%d", i)
		if _, exists := backends[backendName]; !exists {
			t.Errorf("Backend %s not found after concurrent registration", backendName)
		}
	}

	// Cleanup: unregister all backends
	for i := 1; i <= 5; i++ {
		backendName := fmt.Sprintf("concurrent-backend-%d", i)
		if err := c.UnregisterBackend(frontendID, backendName); err != nil {
			t.Errorf("UnregisterBackend(%s) error = %v", backendName, err)
		}
	}
}

// getTestFrontendManager returns the frontend manager for testing
// In a real scenario, this would connect to a running gateway instance
func getTestFrontendManager(t *testing.T) *gateway.FrontendManager {
	// TODO: Implement connection to running gateway instance
	// This could be done via:
	// 1. Shared memory
	// 2. gRPC service
	// 3. Unix domain socket
	// 4. Environment variable pointing to gateway instance

	t.Skip("Frontend manager connection not implemented yet")
	return nil
}
