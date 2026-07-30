package gateway

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
)

// seedConfig is the smallest configuration the parser accepts; it stands in for
// the image's /etc/haproxy/haproxy.cfg.
const seedConfig = `global
	daemon

defaults
	mode http
	timeout connect 5s
	timeout client 30s
	timeout server 30s

frontend http-gateway
	bind *:8080
	default_backend seed_be

backend seed_be
	server SRV_1 127.0.0.1:8081
`

// newTestHAProxyClient builds a real HAProxy client over a throwaway config
// file. The stub binary stands in for "haproxy -c", which client-native runs on
// every transaction it commits.
func newTestHAProxyClient(t *testing.T) (api.HAProxyClient, string) {
	t.Helper()

	dir := t.TempDir()

	cfgFile := filepath.Join(dir, "haproxy.cfg")
	if err := os.WriteFile(cfgFile, []byte(seedConfig), 0o600); err != nil {
		t.Fatalf("write seed config: %v", err)
	}

	txDir := filepath.Join(dir, "transactions")
	if err := os.MkdirAll(txDir, 0o700); err != nil {
		t.Fatalf("create transaction dir: %v", err)
	}

	stub := filepath.Join(dir, "haproxy-stub")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write haproxy stub: %v", err)
	}

	client, err := api.New(txDir, cfgFile, stub, filepath.Join(dir, "runtime.sock"))
	if err != nil {
		t.Fatalf("new haproxy client: %v", err)
	}
	return client, cfgFile
}

// TestConcurrentRegisterBackendAndAddRoute is the incident in miniature: several
// frontends sharing one HAProxy client, with backend registrations and route
// additions arriving concurrently the way a governance notification storm
// delivers them at startup. Both operations open their own transaction, so
// before the client serialized them they clobbered each other's transaction ID
// and the resulting empty-ID commit deleted the live config.
func TestConcurrentRegisterBackendAndAddRoute(t *testing.T) {
	client, cfgFile := newTestHAProxyClient(t)

	ids := []string{"ausf", "udm", "udr"}
	config := FrontendConfig{}
	for i, id := range ids {
		config.Frontends = append(config.Frontends, FrontendDefinition{
			ID:      id,
			Name:    id + "_fe",
			Enabled: true,
			Mode:    "http",
			Bindings: []BindingDefinition{{
				Address:  "0.0.0.0",
				Port:     36010 + i*10,
				Protocol: "http",
				HTTP2:    true,
			}},
		})
	}

	fm := NewFrontendManager(client, config)

	ctx, cancel := context.WithCancel(context.Background())
	// Only the context is cancelled: FrontendManager.Stop calls Manager.Stop,
	// which dereferences the nil provider these Managers are built with.
	defer cancel()

	if err := fm.Start(ctx); err != nil {
		t.Fatalf("start frontends: %v", err)
	}

	var (
		mu   sync.Mutex
		errs []error
	)
	record := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		errs = append(errs, err)
	}

	const rounds = 3

	var wg sync.WaitGroup
	for _, id := range ids {
		for r := 0; r < rounds; r++ {
			wg.Add(1)
			go func(id string, r int) {
				defer wg.Done()
				if err := fm.RegisterBackend(id, Backend{
					Name: id + "_be",
					Servers: []BackendServer{{
						Name: fmt.Sprintf("SRV_%d", r+1),
						IP:   fmt.Sprintf("10.1.%d.%d", len(id), r+1),
						Port: 8080,
					}},
				}); err != nil {
					record(fmt.Errorf("register %s round %d: %w", id, r, err))
				}
			}(id, r)

			wg.Add(1)
			go func(id string, r int) {
				defer wg.Done()
				if err := fm.AddRoute(id, Route{
					ID:          fmt.Sprintf("%s_route_%d", id, r),
					Path:        fmt.Sprintf("/n%s/v%d", id, r+1),
					BackendName: id + "_be",
					FrontendID:  id,
				}); err != nil {
					record(fmt.Errorf("add route %s round %d: %w", id, r, err))
				}
			}(id, r)
		}
	}
	wg.Wait()

	for _, err := range errs {
		t.Errorf("concurrent gateway update failed: %v", err)
	}

	// The live config must still exist and still describe every frontend: the
	// symptom of the incident was this file being deleted, after which no SBI
	// listener could ever be bound.
	data, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatalf("config file must survive concurrent updates: %v", err)
	}
	for _, id := range ids {
		if !strings.Contains(string(data), "frontend "+id+"_fe") {
			t.Errorf("frontend %s_fe missing from config:\n%s", id, data)
		}
		if !strings.Contains(string(data), "backend "+id+"_be") {
			t.Errorf("backend %s_be missing from config:\n%s", id, data)
		}
	}
}
