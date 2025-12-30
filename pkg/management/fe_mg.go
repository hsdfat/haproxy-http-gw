package management

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/haproxytech/kubernetes-ingress/pkg/config/env"
	"github.com/haproxytech/kubernetes-ingress/pkg/gateway"
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/instance"
	"github.com/haproxytech/kubernetes-ingress/pkg/logger"
)

const (
	EirId = "eir"
)

// ReloadCallback is called after HAProxy is successfully reloaded
// It's used to re-register backends that were registered via runtime socket
type ReloadCallback func()

type ConfigExample struct{}

func (c *ConfigExample) LoadConfig() (*gateway.FrontendConfig, error) {
	// Placeholder for loading configuration logic
	return &gateway.FrontendConfig{
		Frontends: []gateway.FrontendDefinition{
			{
				ID:      EirId,
				Name:    "eir-frontend",
				Mode:    "http",
				Enabled: true,
				Bindings: []gateway.BindingDefinition{
					{
						Address:  "0.0.0.0",
						Port:     env.Env.ServicePort,
						Protocol: "http",
						HTTP2:    true,
					},
				},
				Routing: gateway.RoutingConfig{
					BypassRules:    false,
					DefaultBackend: EirId + "_be",
				},
				Options: gateway.FrontendOptions{
					MaxConnections:     10000,
					TimeoutClient:      30,
					HTTPConnectionMode: "http-keep-alive",
				},
			},
		},
	}, nil
}

func (c *ConfigExample) ValidateConfig(cfg *gateway.FrontendConfig) error {
	// Placeholder for configuration validation logic
	return nil
}

func (c *ConfigExample) GetName() string {

	return "EIR Gateway Configuration"
}

var FM *gateway.FrontendManager

func Init(client api.HAProxyClient) {
	log := logger.Haproxy
	registry := gateway.NewConfigRegistry()

	registry.Register(&ConfigExample{})
	config, err := registry.LoadConfig()
	if err != nil {
		log.Fatalf("Error loading frontend config: %v", err)
	}

	for _, fe := range config.Frontends {
		log.Infof("Loaded frontend: %s", fe.Name)
	}

	FM = gateway.NewFrontendManager(client, *config)
}

func InitHaproxyConfig(reloadCallback ReloadCallback) {

	log := logger.Haproxy
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := FM.Start(ctx); err != nil {
		log.Fatalf("Error starting Frontend Manager: %v", err)
	}

	log.Info("Frontend Manager started successfully")

	apiPort := 9090
	enhancedAPI := gateway.NewEnhancedAPIServer(FM, apiPort)
	addDefautltBackendRoute(FM, EirId)

	// Trigger immediate reload to activate the newly created frontend
	time.Sleep(500 * time.Millisecond) // Brief delay to ensure frontend is written to config
	if err := reloadHaproxyConfig(); err != nil {
		log.Errorf("Failed to reload HAProxy after frontend creation: %v", err)
	} else {
		log.Info("HAProxy reloaded successfully after frontend creation")
		// Call the callback to re-register backends after initial reload
		if reloadCallback != nil {
			log.Info("Executing reload callback to restore backend state...")
			reloadCallback()
		}
	}

	go func() {
		if err := enhancedAPI.Start(); err != nil {
			log.Errorf("Error starting Enhanced API Server: %v", err)
		}
	}()

	go monitorHaproxyReload(ctx, reloadCallback)

	sigChan := make(chan os.Signal, 1)
	go func() {
		<-sigChan
		log.Info("Shutting down Enhanced API Server...")
		enhancedAPI.Stop()
		cancel()
		FM.Stop()
	}()
}

func addDefautltBackendRoute(fm *gateway.FrontendManager, id string) {
	routeId := id + "_route"
	beId := id + "_be"

	fm.RegisterBackend(id, gateway.Backend{
		Name:    beId,
		Servers: []gateway.BackendServer{},
	})

	fm.AddRoute(id, gateway.Route{
		ID:          routeId,
		Host:        "",
		Path:        "/",
		BackendName: beId,
		FrontendID:  id,
	})
	logger.Haproxy.Infow("Added default backend route",
		"frontend", id,
		"route", routeId,
		"backend", beId,
	)
}

func monitorHaproxyReload(ctx context.Context, reloadCallback ReloadCallback) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if instance.NeedReload() {
				logger.Haproxy.Info("HAProxy configuration changed, reloading...")
				if err := reloadHaproxyConfig(); err != nil {
					logger.Haproxy.Errorf("Failed to reload HAProxy: %v", err)
				} else {
					logger.Haproxy.Info("HAProxy reloaded successfully")
					instance.Reset()
					// Call the callback to re-register backends after reload
					if reloadCallback != nil {
						logger.Haproxy.Info("Executing reload callback to restore backend state...")
						reloadCallback()
					}
				}
			}
		case <-ctx.Done():
			logger.Haproxy.Info("Stopping HAProxy reload monitor")
			return
		}
	}
}

func reloadHaproxyConfig() error {
	logger.Haproxy.Info("Reloading HAProxy configuration...")
	haproxyBin := env.Env.HaproxyBinary
	configFile := env.Env.ConfigFile

	validateCmd := exec.Command(haproxyBin, "-c", "-f", configFile)
	if output, err := validateCmd.CombinedOutput(); err != nil {
		logger.Haproxy.Errorf("HAProxy configuration validation failed: %s", string(output))
		return err
	}

	pidFile := env.Env.HaproxyPIDFile
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		logger.Haproxy.Errorf("Failed to read HAProxy PID file: %v", err)
		return err
	}

	pidStr := string(pidData)
	pidStr = string([]rune(pidStr)[:len(pidStr)-1]) // Remove newline if present
	logger.Haproxy.Infof("Current HAProxy PID: %s", pidStr)
	logger.Haproxy.Infow("Sending SIGUSR2 to HAProxy for reload", "pid", pidStr)
	killCmd := exec.Command("kill", "-USR2", pidStr)
	if output, err := killCmd.CombinedOutput(); err != nil {
		logger.Haproxy.Errorf("Failed to send SIGUSR2 to HAProxy: %s", string(output))
		return err
	}
	logger.Haproxy.Info("HAProxy reloaded successfully")
	return nil
}
