package main

import (
	"os"
	"os/signal"
	"syscall"

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
	log := logger.Log.With("app", "http-gw").(logger.Logger)
	env.Init()

	runtimeSocketPath := env.Env.RuntimeSocket
	log.Info("Runtime Socket Path: ", runtimeSocketPath)

	haproxyClient, err := api.New(env.Env.TransactionDir,
		env.Env.ConfigFile,
		env.Env.HaproxyBinary,
		runtimeSocketPath,
	)
	if err != nil {
		log.Fatal("Failed to create HAProxy client: ", err)
	}
	// Use haproxyClient as needed...
	management.Init(haproxyClient)
	var govClient *govclient.Client
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
		management.FM.RegisterBackend(management.EirId, gateway.Backend{
			Name:    management.EirId + "_be",
			Servers: bes,
		})
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
