package governance

import (
	"fmt"
	"os"
	"time"

	govclient "github.com/chronnie/governance/client"
	"github.com/chronnie/governance/models"
	"github.com/haproxytech/kubernetes-ingress/pkg/config/env"
	"github.com/haproxytech/kubernetes-ingress/pkg/logger"
)

func RegisterWithGovernance(log logger.Logger, notifyMiddleware func(payload *models.NotificationPayload)) *govclient.Client {
	config := env.Env
	governanceURL := config.TargetGov

	podName, _ := os.Hostname()

	// Create governance client
	govClient := govclient.NewClient(&govclient.ClientConfig{
		ManagerURL:  "http://" + governanceURL,
		ServiceName: models.ServiceNameHttpGw,
		PodName:     podName,
	})
	govClient.NotifyFunc = notifyMiddleware

	// Use standard governance endpoints: /health for health checks and /notify for notifications
	// Port is controlled by GOV_BACKEND_PORT environment variable (default 2345)
	go govClient.StartHTTPServerWithClient(govclient.HTTPServerConfig{
		Port: config.GovBackendPort,
	})

	// Wait a bit for server to start
	time.Sleep(200 * time.Millisecond)

	// Register http-gw service and subscribe to configured services
	registration := &models.ServiceRegistration{
		ServiceName: models.ServiceNameHttpGw,
		PodName:     podName,
		Providers: []models.ProviderInfo{
			{
				ProviderID: "http",
				Protocol:   models.ProtocolHTTP,
				IP:         config.ServiceIP,
				Port:       config.ServicePort,
			},
		},
		HealthCheckURL:  fmt.Sprintf("http://%s:%d/health", config.ServiceIP, config.GovBackendPort),
		NotificationURL: fmt.Sprintf("http://%s:%d/notify", config.ServiceIP, config.GovBackendPort),
		Subscriptions: []models.Subscription{{
			ServiceName: models.ServiceNameEir,
			ProviderIDs: []string{string(models.ProviderEIRHTTP)},
		}},
	}

	resp, err := govClient.Register(registration)
	if err != nil {
		log.Warnw("Failed to register with governance manager", "error", err)
		panic(err)
	} else {
		log.Infow("✓ Registered with governance manager",
			"url", governanceURL,
			"service", models.ServiceNameHttpGw,
			"pod", podName,
			"own_pods", len(resp.Pods),
			"subscribed_services", len(resp.SubscribedServices))

		// Log subscription details
		for svcName, pods := range resp.SubscribedServices {
			log.Infow("  Subscription", "service", svcName, "pods", len(pods))
		}
	}

	// Start heartbeat
	govClient.StartHeartbeat()
	log.Infow("✓ Started governance heartbeat")

	return govClient
}
