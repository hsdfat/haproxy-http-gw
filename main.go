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

//nolint:forbidigo
package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	//nolint:gosec
	_ "net/http/pprof"

	"k8s.io/apimachinery/pkg/watch"

	"github.com/go-test/deep"
	"github.com/jessevdk/go-flags"

	convert "github.com/haproxytech/kubernetes-ingress/crs/converters/v1v3"
	"github.com/haproxytech/kubernetes-ingress/pkg/annotations"
	"github.com/haproxytech/kubernetes-ingress/pkg/controller"
	"github.com/haproxytech/kubernetes-ingress/pkg/job"
	"github.com/haproxytech/kubernetes-ingress/pkg/k8s"
	"github.com/haproxytech/kubernetes-ingress/pkg/k8s/meta"
	k8ssync "github.com/haproxytech/kubernetes-ingress/pkg/k8s/sync"
	"github.com/haproxytech/kubernetes-ingress/pkg/store"
	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
	"github.com/haproxytech/kubernetes-ingress/pkg/version"

	_ "go.uber.org/automaxprocs"
)

//go:embed fs/usr/local/etc/haproxy/haproxy.cfg
var haproxyConf []byte

func main() {
	exitCode := 0
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Error : ", r)
		}
		os.Exit(exitCode)
	}()

	// Parse Controller Args
	var osArgs utils.OSArgs
	var err error
	parser := flags.NewParser(&osArgs, flags.IgnoreUnknown)
	if _, err = parser.Parse(); err != nil {
		fmt.Println(err)
		exitCode = 1
		return
	}

	// Set Logger
	logger := utils.GetLogger()
	err = version.Set()
	if err != nil {
		logger.Error(err)
		os.Exit(1) //nolint:gocritic
	}

	// logger.SetLevel(osArgs.LogLevel.LogLevel)
	if len(osArgs.Help) > 0 && osArgs.Help[0] {
		parser.WriteHelp(os.Stdout)
		return
	}

	if osArgs.JobCheckCRD {
		logger.Debug(version.IngressControllerInfo)
		logger.Debug(job.IngressControllerCRDUpdater)
		logger.Infof("HAProxy Ingress Controller CRD Updater %s %s", version.GitTag, version.GitCommit)
		logger.Infof("Build from: %s", version.GitRepo)
		logger.Infof("Build date: %s\n", version.GitCommitDate)

		err := job.CRDRefresh(logger, osArgs)
		if err != nil {
			logger.Error(err)
			os.Exit(1)
		}
		// exit, this is just a job
		os.Exit(0)
	}

	if osArgs.CRDInputFile != "" && osArgs.CRDOutputFile != "" {
		logger.Debug(version.IngressControllerInfo)
		logger.Infof("HAProxy Ingress Controller CRD Converter %s %s", version.GitTag, version.GitCommit)
		logger.Infof("Build from: %s", version.GitRepo)
		logger.Infof("Build date: %s\n", version.GitCommitDate)

		err := convert.ConvertV1V3(osArgs)
		if err != nil {
			logger.Error(err)
			os.Exit(1)
		}
		// exit, this is just a job
		os.Exit(0)
	}

	if osArgs.InitialSyncPeriod == 0 {
		osArgs.InitialSyncPeriod = osArgs.SyncPeriod
	}

	// logger.ShowFilename(false)
	exit := logInfo(logger, osArgs)
	if exit {
		return
	}
	// logger.ShowFilename(true)

	annotations.DisableConfigSnippets(osArgs.DisableConfigSnippets)

	// backwards compatibility with 1.7
	if osArgs.PprofEnabled && osArgs.ControllerPort == 0 {
		osArgs.ControllerPort = 6060
	}
	if osArgs.PrometheusEnabled && osArgs.ControllerPort == 0 {
		osArgs.ControllerPort = 6060
	}

	deep.NilMapsAreEmpty = true
	deep.NilSlicesAreEmpty = true

	// Default annotations
	defaultBackendSvc := fmt.Sprint(osArgs.DefaultBackendService)
	defaultCertificate := fmt.Sprint(osArgs.DefaultCertificate)
	annotations.SetDefaultValue("default-backend-service", defaultBackendSvc)
	annotations.SetDefaultValue("default-backend-port", strconv.Itoa(osArgs.DefaultBackendPort))
	annotations.SetDefaultValue("ssl-certificate", defaultCertificate)

	// Start Controller
	var chanSize int64 = int64(watch.DefaultChanSize * 6)
	if osArgs.ChannelSize > 0 {
		chanSize = osArgs.ChannelSize
	}
	eventChan := make(chan k8ssync.SyncDataEvent, chanSize)
	stop := make(chan struct{})

	publishService := getNamespaceValue(osArgs.PublishService)

	s := store.NewK8sStore(osArgs)
	k := k8s.New(
		osArgs,
		s.NamespacesAccess.Whitelist,
		publishService,
	)

	if osArgs.Test {
		meta.GetMetaStore().ProcessedResourceVersion.SetTestMode()
	}

	c := controller.NewBuilder().
		WithHaproxyCfgFile(haproxyConf).
		WithEventChan(eventChan).
		WithStore(s).
		WithClientSet(k.GetClientset()).
		WithRestClientSet(k.GetRestClientset()).
		WithArgs(osArgs).Build()

	isGatewayAPIInstalled := k.IsGatewayAPIInstalled(osArgs.GatewayControllerName)

	c.SetGatewayAPIInstalled(isGatewayAPIInstalled)

	go k.MonitorChanges(eventChan, stop, osArgs, isGatewayAPIInstalled)
	go c.Start()
	// Catch QUIT signals
	signalC := make(chan os.Signal, 1)
	signal.Notify(signalC, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR1)
	<-signalC
	logger.Debug("Graceful shutdown requested ....")
	c.Stop()
	close(stop)
	logger.Debug("Graceful shutdown done, exiting")
}

func logInfo(logger utils.Logger, osArgs utils.OSArgs) bool {
	if len(osArgs.Version) > 0 {
		fmt.Printf("HAProxy Ingress Controller %s %s\n", version.GitTag, version.GitCommit)
		fmt.Printf("Build from: %s\n", version.GitRepo)
		fmt.Printf("Git commit date: %s\n", version.GitCommitDate)
		if len(osArgs.Version) > 1 {
			fmt.Printf("ConfigMap: %s\n", osArgs.ConfigMap)
			fmt.Printf("Ingress class: %s\n", osArgs.IngressClass)
			fmt.Printf("Empty Ingress class: %t\n", osArgs.EmptyIngressClass)
		}
		if osArgs.Experimental.UseIngressMerge {
			fmt.Println("Ingress Management: merge implementation")
		} else {
			fmt.Println("Ingress Management: default implementation")
		}
		return true
	}

	logger.Debug(version.IngressControllerInfo)
	logger.Debugf("HAProxy Ingress Controller %s %s", version.GitTag, version.GitCommit)
	logger.Debugf("Build from: %s", version.GitRepo)
	logger.Debugf("Git commit date: %s", version.GitCommitDate)
	logger.Debugf("ConfigMap: %s", osArgs.ConfigMap)
	logger.Debugf("Ingress class: %s", osArgs.IngressClass)
	logger.Debugf("Empty Ingress class: %t", osArgs.EmptyIngressClass)
	if osArgs.GatewayControllerName != "" {
		// display log message only if Gateway API is used
		logger.Debugf("Gateway API controller name: %s", osArgs.GatewayControllerName)
	}
	logger.Debugf("Publish service: %s", osArgs.PublishService)
	if osArgs.DefaultBackendService.String() != "" {
		logger.Debugf("Default backend service: %s", osArgs.DefaultBackendService)
	} else {
		logger.Debugf("Using local backend service on port: %d", osArgs.DefaultBackendPort)
	}
	logger.Debugf("Default ssl certificate: %s", osArgs.DefaultCertificate)
	if !osArgs.DisableHTTP {
		logger.Debugf("Frontend HTTP listening on: %s:%d", osArgs.IPV4BindAddr, osArgs.HTTPBindPort)
	}
	if !osArgs.DisableHTTPS {
		logger.Debugf("Frontend HTTPS listening on: %s:%d", osArgs.IPV4BindAddr, osArgs.HTTPSBindPort)
	}
	if osArgs.DisableHTTP {
		logger.Debugf("Disabling HTTP frontend")
	}
	if osArgs.DisableHTTPS {
		logger.Debugf("Disabling HTTPS frontend")
	}
	if osArgs.DisableIPV4 {
		logger.Debugf("Disabling IPv4 support")
	}
	if osArgs.DisableIPV6 {
		logger.Debugf("Disabling IPv6 support")
	}
	if osArgs.ConfigMapTCPServices.Name != "" {
		logger.Debugf("TCP Services provided in '%s'", osArgs.ConfigMapTCPServices)
	}
	if osArgs.ConfigMapErrorFiles.Name != "" {
		logger.Debugf("Errorfiles provided in '%s'", osArgs.ConfigMapErrorFiles)
	}
	if osArgs.ConfigMapPatternFiles.Name != "" {
		logger.Debugf("Pattern files provided in '%s'", osArgs.ConfigMapPatternFiles)
	}
	if osArgs.DisableConfigSnippets != "" {
		logger.Debugf("Disabling config snippets for [%s]", osArgs.DisableConfigSnippets)
	}
	if osArgs.DisableDelayedWritingOnlyIfReload {
		logger.Debugf("Disabling the delayed writing of files to disk only in case of haproxy reload (write to disk even if no reload)")
	}
	logger.Debugf("Kubernetes Informers resync period: %s", osArgs.CacheResyncPeriod.String())
	logger.Debugf("Controller initial sync period: %s", osArgs.InitialSyncPeriod.String())
	if osArgs.Experimental.UseIngressMerge {
		logger.Debug("Ingress Management: merge implementation")
	} else {
		logger.Debug("Ingress Management: default implementation")
	}
	logger.Debugf("Controller sync period: %s\n", osArgs.SyncPeriod.String())
	hostname, err := os.Hostname()
	logger.Error(err)
	logger.Debugf("Running on %s", hostname)
	return false
}

func getNamespaceValue(name string) *utils.NamespaceValue {
	parts := strings.Split(name, "/")
	var result *utils.NamespaceValue
	if len(parts) == 2 {
		result = &utils.NamespaceValue{
			Namespace: parts[0],
			Name:      parts[1],
		}
	}
	return result
}
