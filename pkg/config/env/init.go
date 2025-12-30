package env

import (
	"github.com/haproxytech/kubernetes-ingress/pkg/logger"
	"github.com/hsdfat/telco/envconfig"
)

var Env EnvConfigs

func Init(log logger.Logger) {
	SetLogger(log)
	err := envconfig.ReadConfigFrom("", &Env)
	if err != nil {
		log.Fatalf("Error loading env config: %v", err)
	}
}
