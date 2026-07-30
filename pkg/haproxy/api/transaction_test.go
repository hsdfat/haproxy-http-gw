package api

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/haproxytech/client-native/v6/models"
	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

// seedConfig is the smallest configuration the parser and the transaction
// machinery accept; it stands in for the image's /etc/haproxy/haproxy.cfg.
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

// newTestClient builds a real client over a throwaway configuration file. The
// stub binary stands in for "haproxy -c": client-native validates every
// transaction file it commits, and this host has no haproxy.
func newTestClient(t *testing.T) (HAProxyClient, string) {
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

	client, err := New(txDir, cfgFile, stub, filepath.Join(dir, "runtime.sock"))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client, cfgFile
}

// TestCommitWithoutTransactionKeepsConfigFile is the regression test for the
// 2026-07-29 incident: client-native resolves an empty transaction ID to the
// live configuration file and deletes it on the commit success path, so both
// commit APIs must refuse an empty ID outright.
func TestCommitWithoutTransactionKeepsConfigFile(t *testing.T) {
	client, cfgFile := newTestClient(t)

	before, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatalf("read config before: %v", err)
	}

	assertRefused := func(step string, err error) {
		t.Helper()
		if !errors.Is(err, ErrNoActiveTransaction) {
			t.Fatalf("%s: err = %v, want ErrNoActiveTransaction", step, err)
		}
	}

	// Never started.
	assertRefused("commit before start", client.APICommitTransaction())
	assertRefused("final commit before start", client.APIFinalCommitTransaction())

	// Disposing a transaction that was never started must not panic on an
	// unlocked mutex, and must leave the client usable.
	client.APIDisposeTransaction()

	// The incident's shape: a transaction with staged changes whose ID another
	// goroutine's dispose cleared. The staged changes make the configuration hash
	// differ, which is what carries the commit past the no-op shortcut and into
	// client-native's CommitTransaction -- and with an empty ID that deletes the
	// live config file on its success path.
	// Dispose is deferred immediately, as production callers must; the explicit
	// dispose below then plays the racing goroutine, and the deferred one is the
	// no-op double dispose.
	if err := client.APIStartTransaction(); err != nil {
		t.Fatalf("start transaction: %v", err)
	}
	defer client.APIDisposeTransaction()
	client.BackendCreatePermanently(models.Backend{
		BackendBase: models.BackendBase{Name: "staged_be", Mode: "http"},
	})
	if err := client.BackendServerCreate("staged_be", models.Server{
		Name:    "SRV_1",
		Address: "10.0.0.9",
		Port:    utils.PtrInt64(8080),
	}); err != nil {
		t.Fatalf("create staged server: %v", err)
	}
	client.APIDisposeTransaction()

	assertRefused("commit after dispose", client.APICommitTransaction())
	assertRefused("final commit after dispose", client.APIFinalCommitTransaction())

	after, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatalf("config file must survive a commit with no transaction: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("config file changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestRegisterAndCommitTransaction checks the normal path still works: a
// started transaction commits its staged backend into the live config.
func TestRegisterAndCommitTransaction(t *testing.T) {
	client, cfgFile := newTestClient(t)

	if err := client.APIStartTransaction(); err != nil {
		t.Fatalf("start transaction: %v", err)
	}
	defer client.APIDisposeTransaction()

	client.BackendCreatePermanently(models.Backend{
		BackendBase: models.BackendBase{Name: "udm_be", Mode: "http"},
	})
	if err := client.BackendServerCreate("udm_be", models.Server{
		Name:    "SRV_1",
		Address: "10.0.0.1",
		Port:    utils.PtrInt64(8080),
	}); err != nil {
		t.Fatalf("create server: %v", err)
	}

	if err := client.APIFinalCommitTransaction(); err != nil {
		t.Fatalf("final commit: %v", err)
	}
	client.APIDisposeTransaction()

	data, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatalf("read config after commit: %v", err)
	}
	if !strings.Contains(string(data), "udm_be") {
		t.Fatalf("committed backend missing from config:\n%s", data)
	}

	// The client is usable again: the transaction lock was released by dispose.
	if err := client.APIStartTransaction(); err != nil {
		t.Fatalf("start second transaction: %v", err)
	}
	client.APIDisposeTransaction()
}

// TestStrayDisposeDoesNotStealTransaction pins the deferred-dispose
// interleaving: goroutine A starts and disposes a transaction, goroutine B
// starts its own, and then A's deferred dispose fires. With ownership tracked
// as a plain boolean that stray dispose cleared B's transaction ID and
// unlocked B's mutex mid-flight; with per-goroutine ownership it must be a
// no-op, and B's commit must still succeed. A non-owner commit attempt must be
// refused rather than committing B's transaction.
func TestStrayDisposeDoesNotStealTransaction(t *testing.T) {
	client, _ := newTestClient(t)

	// Goroutine A: start and dispose, leaving a "deferred" dispose to fire late.
	if err := client.APIStartTransaction(); err != nil {
		t.Fatalf("A: start transaction: %v", err)
	}
	client.APIDisposeTransaction()

	bStarted := make(chan struct{})
	strayDone := make(chan struct{})
	bResult := make(chan error, 1)

	go func() {
		// Goroutine B: own transaction with staged work.
		if err := client.APIStartTransaction(); err != nil {
			bResult <- fmt.Errorf("B: start transaction: %w", err)
			return
		}
		defer client.APIDisposeTransaction()
		client.BackendCreatePermanently(models.Backend{
			BackendBase: models.BackendBase{Name: "b_be", Mode: "http"},
		})
		close(bStarted)
		<-strayDone
		bResult <- client.APIFinalCommitTransaction()
	}()

	<-bStarted
	// A's deferred dispose fires while B's transaction is active: must be a
	// no-op. A commit from this goroutine must be refused — it is not the owner.
	client.APIDisposeTransaction()
	if err := client.APICommitTransaction(); !errors.Is(err, ErrNotTransactionOwner) {
		t.Errorf("non-owner commit: err = %v, want ErrNotTransactionOwner", err)
	}
	close(strayDone)

	if err := <-bResult; err != nil {
		t.Fatalf("B's commit must survive the stray dispose: %v", err)
	}
}

// TestConcurrentTransactions runs the startup storm that bricked the pod: many
// goroutines starting, staging and committing transactions on one shared
// client. Any interleaving used to erase another goroutine's transaction ID,
// which surfaced as an empty-ID commit and cost the live config file.
func TestConcurrentTransactions(t *testing.T) {
	client, cfgFile := newTestClient(t)

	const (
		goroutines = 8
		rounds     = 4
	)

	var (
		mu   sync.Mutex
		errs []error
	)
	record := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		errs = append(errs, err)
	}

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				if err := client.APIStartTransaction(); err != nil {
					record(fmt.Errorf("g%d r%d start: %w", g, r, err))
					continue
				}
				name := fmt.Sprintf("be_%d", g)
				client.BackendCreatePermanently(models.Backend{
					BackendBase: models.BackendBase{Name: name, Mode: "http"},
				})
				if err := client.BackendServerCreateOrUpdate(name, models.Server{
					Name:    fmt.Sprintf("SRV_%d", r+1),
					Address: fmt.Sprintf("10.0.%d.%d", g, r+1),
					Port:    utils.PtrInt64(8080),
				}); err != nil {
					record(fmt.Errorf("g%d r%d server: %w", g, r, err))
				}
				if err := client.APIFinalCommitTransaction(); err != nil {
					record(fmt.Errorf("g%d r%d commit: %w", g, r, err))
				}
				client.APIDisposeTransaction()
			}
		}(g)
	}
	wg.Wait()

	for _, err := range errs {
		t.Errorf("concurrent transaction failed: %v", err)
	}

	// The live config must still be there, and still be the live config.
	data, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatalf("config file must survive concurrent transactions: %v", err)
	}
	if !strings.Contains(string(data), "frontend http-gateway") {
		t.Fatalf("config file no longer holds the seed frontend:\n%s", data)
	}
	for g := 0; g < goroutines; g++ {
		name := fmt.Sprintf("be_%d", g)
		if !strings.Contains(string(data), name) {
			t.Errorf("backend %s missing from committed config", name)
		}
	}
}
