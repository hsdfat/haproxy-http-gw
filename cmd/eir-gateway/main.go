package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	govclient "github.com/chronnie/governance/client"
	"github.com/chronnie/governance/models"
	"github.com/haproxytech/kubernetes-ingress/pkg/config/env"
	"github.com/haproxytech/kubernetes-ingress/pkg/gateway"
	"github.com/haproxytech/kubernetes-ingress/pkg/governance"
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
	"github.com/haproxytech/kubernetes-ingress/pkg/logger"
	"github.com/haproxytech/kubernetes-ingress/pkg/management"
)

func main() {
	log := logger.New("http-gw", "info")
	env.Init(log)

	runtimeSocketPath := env.Env.RuntimeSocket
	log.Info("Runtime Socket Path: ", runtimeSocketPath)

	mapsDir := env.Env.HaproxyMapsDir
	if mapsDir == "" {
		mapsDir = "/etc/haproxy/maps"
	}
	haproxyClient, err := api.New(env.Env.TransactionDir,
		env.Env.ConfigFile,
		env.Env.HaproxyBinary,
		runtimeSocketPath,
		mapsDir,
	)
	if err != nil {
		log.Fatal("Failed to create HAProxy client: ", err)
	}
	// Use haproxyClient as needed...
	management.Init(haproxyClient)

	var govClient *govclient.Client

	// Create a callback function for re-registering backends after HAProxy reload
	// This fetches the current pod list from governance and re-registers all backends
	reloadCallback := func() {
		time.Sleep(time.Second)
		if !env.Env.Governance {
			log.Info("Governance disabled, skipping backend re-registration after reload")
			return
		}

		if govClient == nil {
			log.Warn("Governance client not initialized, cannot re-register backends after reload")
			return
		}

		log.Info("Re-registering backends after HAProxy reload...")

		// Fetch current pod list from governance for EIR service
		pods := govClient.GetPodInfos(string(models.ServiceNameEir), string(models.ProviderEIRHTTP))
		if pods == nil || len(pods) == 0 {
			log.Warn("No pods found in governance for re-registration")
			return
		}

		// Convert pods to backend servers
		bes := make([]gateway.BackendServer, 0, len(pods))
		for _, pod := range pods {
			bes = append(bes, gateway.BackendServer{
				Name: pod.Name,
				IP:   pod.Ip,
				Port: int(pod.Port),
			})
		}

		log.Infow("Re-registering EIR backend after reload", "server_count", len(bes))

		// Re-register the backend
		err := management.FM.RegisterBackend(management.EirId, gateway.Backend{
			Name:    management.EirId + "_be",
			Servers: bes,
		})
		if err != nil {
			log.Errorw("Failed to re-register backend after reload", "error", err)
			return
		}

		log.Infow("Successfully re-registered backend after reload", "backend", management.EirId+"_be", "servers", len(bes))
	}

	// Start HAProxy config initialization with reload callback
	go management.InitHaproxyConfig(reloadCallback)

	var notifyMiddleware func(payload *models.NotificationPayload) = func(payload *models.NotificationPayload) {
		log.Infow("Received governance notification",
			"service_name", payload.ServiceName)
		if payload.ServiceName != models.ServiceNameEir {
			log.Warnw("Ignoring notification for unsupported service",
				"service_name", payload.ServiceName)
			return
		}
		bes := make([]gateway.BackendServer, 0, len(payload.Pods))
		pods, err := govclient.GetPodInfos(*payload, models.ServiceNameEir, string(models.ProviderEIRHTTP))
		if err != nil {
			log.Errorw("Failed to get pod infos from notification payload", "error", err)
			return
		}
		for _, pod := range pods {
			bes = append(bes, gateway.BackendServer{
				Name: pod.Name,
				IP:   pod.Ip,
				Port: int(pod.Port),
			})
		}
		log.Infow("update eir backend", "backend", bes)
		err = management.FM.RegisterBackend(management.EirId, gateway.Backend{
			Name:    management.EirId + "_be",
			Servers: bes,
		})
		if err != nil {
			log.Errorw("Failed to update backend servers from notification", "error", err)
			return
		}
		// Handle the notification as needed...
	}
	if env.Env.Governance {
		govClient = governance.RegisterWithGovernance(logger.Log, notifyMiddleware)
	}

	// Register with governance manager if enabled

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Infow("Shutting down gracefully...")

	// Unregister from governance
	if govClient != nil {
		govClient.StopHeartbeat()
		if err := govClient.Unregister(); err != nil {
			log.Error("Failed to unregister from governance", "error", err)
		} else {
			log.Infow("✓ Unregistered from governance manager")
		}
	}
}
