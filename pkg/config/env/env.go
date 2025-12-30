package env

import (
	"net"

	"github.com/haproxytech/kubernetes-ingress/pkg/logger"
	"github.com/hsdfat/telco/envconfig"
)

type EnvConfigs struct {
	RuntimeSocket    string `mapstructure:"HAPROXY_RUNTIME_SOCKET"`
	TransactionDir   string `mapstructure:"HAPROXY_TRANSACTION_DIR"`
	ConfigFile       string `mapstructure:"HAPROXY_CONFIG_FILE"`
	HaproxyBinary    string `mapstructure:"HAPROXY_BINARY"`
	HaproxyPIDFile   string `mapstructure:"HAPROXY_PID_FILE"`
	ConfigAddr       string `mapstructure:"CONFIG_ADDR"`
	TargetGov        string `mapstructure:"TARGET_GOV"`
	GovBackendPort   int    `mapstructure:"GOV_BACKEND_PORT"`
	Governance       bool   `mapstructure:"GOVERNANCE"`
	ServiceIP        string `mapstructure:"SERVICE_IP"`
	ServicePort      int    `mapstructure:"SERVICE_PORT"`
	ConfdExpose      bool   `mapstructure:"CONFD_EXPOSE"`
	ConfdServiceAddr string `mapstructure:"CONFD_SERVICE_ADDR"`
}

func (e *EnvConfigs) DefaultValues() {
	e.RuntimeSocket = "/var/run/haproxy-runtime-api.sock"
	e.TransactionDir = "/tmp/haproxy-gateway"
	e.ConfigFile = "/etc/haproxy/haproxy.cfg"
	e.HaproxyBinary = "/usr/local/sbin/haproxy"
	e.HaproxyPIDFile = "/tmp/haproxy-gateway/haproxy.pid"
	e.TargetGov = "127.0.0.1:36610"
	e.Governance = true
	e.GovBackendPort = 2345
	e.ServiceIP = GetLocalIP()
	e.ServicePort = 18200
	e.ConfdExpose = false
}

func (e *EnvConfigs) Print() {
	// No-op, handled elsewhere
	// Logger will be set via SetLogger before calling Print
	if configLogger != nil {
		envconfig.Show(configLogger.With("config", "env").(logger.Logger), e)
	} else {
		envconfig.Show(logger.Log.With("config", "env").(logger.Logger), e)
	}
}

var configLogger logger.Logger

func SetLogger(log logger.Logger) {
	configLogger = log
}

func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "0.0.0.0"
}
