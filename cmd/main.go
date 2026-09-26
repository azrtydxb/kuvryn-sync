/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"os"
	"strings"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	corev1alpha1 "github.com/azrtydxb/kuvryn-sync/api/v1alpha1"
	"github.com/azrtydxb/kuvryn-sync/internal/cli"
	"github.com/azrtydxb/kuvryn-sync/internal/controller"
	"github.com/azrtydxb/kuvryn-sync/internal/imagepolicy"
	"github.com/azrtydxb/kuvryn-sync/internal/imageupdate"
	"github.com/azrtydxb/kuvryn-sync/internal/impersonate"
	"github.com/azrtydxb/kuvryn-sync/internal/notify"
	"github.com/azrtydxb/kuvryn-sync/internal/ops"
	"github.com/azrtydxb/kuvryn-sync/internal/receiver"
	"github.com/azrtydxb/kuvryn-sync/internal/version"
	webhookv1alpha1 "github.com/azrtydxb/kuvryn-sync/internal/webhook/v1alpha1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(corev1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	if handled, code := cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}

	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var defaultServiceAccount string
	var driftResyncInterval time.Duration
	var receiverAddr string
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&defaultServiceAccount, "default-service-account", "",
		"Service account in the Application namespace that Kuvryn Sync impersonates when an Application sets no "+
			"serviceAccountName. When empty, such Applications are refused.")
	flag.DurationVar(&driftResyncInterval, "drift-resync-interval", 5*time.Minute,
		"How often Applications managing kinds the controller may not watch are re-checked for drift. 0 disables it.")
	flag.StringVar(&receiverAddr, "webhook-receiver-bind-address", "",
		"Address for the GitHub/GitLab push webhook receiver, e.g. :9292. Empty disables it.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if defaultServiceAccount != "" {
		if errs := validation.IsDNS1123Subdomain(defaultServiceAccount); len(errs) > 0 {
			err := errors.New(strings.Join(errs, "; "))
			setupLog.Error(err, "Invalid default service account", "name", defaultServiceAccount)
			os.Exit(1)
		}
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling HTTP/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.3/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "kuvryn-sync.kuvryn.io",
		// Git credential Secrets are read directly so the controller never
		// caches every Secret in the cluster.
		Client: client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}}},
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		os.Exit(1)
	}

	// One cache serves both controllers, so one lock guards each repository.
	sourceCache := controller.NewSourceCache()
	if err := mgr.Add(&controller.SourceCachePruner{Client: mgr.GetClient(), Cache: sourceCache}); err != nil {
		setupLog.Error(err, "Failed to add source cache pruner")
		os.Exit(1)
	}
	if err := (&controller.RepositoryReconciler{
		Client:         mgr.GetClient(),
		Scheme:         mgr.GetScheme(),
		SourceResolver: sourceCache,
		ImageUpdater:   &imageupdate.Updater{},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "repository")
		os.Exit(1)
	}
	if receiverAddr != "" {
		if err := mgr.Add(&receiver.Receiver{Client: mgr.GetClient(), Addr: receiverAddr}); err != nil {
			setupLog.Error(err, "Failed to add webhook receiver")
			os.Exit(1)
		}
	}
	notifier := notify.NewDispatcher(nil, 1000)
	if err := mgr.Add(notifier); err != nil {
		setupLog.Error(err, "Failed to add notification dispatcher")
		os.Exit(1)
	}
	if err := (&controller.ApplicationReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Tracer: ops.NewOTelTracer("github.com/azrtydxb/kuvryn-sync/controller"),
		Impersonation: impersonate.New(mgr.GetConfig(), client.Options{
			Scheme: mgr.GetScheme(),
			Mapper: mgr.GetRESTMapper(),
		}),
		DefaultServiceAccount: defaultServiceAccount,
		DriftResyncInterval:   driftResyncInterval,
		Notifier:              notifier,
		SourceResolver:        sourceCache,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "application")
		os.Exit(1)
	}
	if err := (&controller.RevisionReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "revision")
		os.Exit(1)
	}
	// nolint:goconst
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err := webhookv1alpha1.SetupHealthCheckWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to create webhook", "webhook", "HealthCheck")
			os.Exit(1)
		}
		if err := webhookv1alpha1.SetupApplicationWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to create webhook", "webhook", "Application")
			os.Exit(1)
		}
		if err := mgr.Add(&controller.RollbackRequestAudit{Reader: mgr.GetAPIReader()}); err != nil {
			setupLog.Error(err, "Failed to add the rollback request audit")
			os.Exit(1)
		}
	} else {
		setupLog.Info("Admission webhooks are disabled; manual approval and rollback request records are not verified " +
			"and can be forged by anyone who can update an Application")
	}
	if err := (&controller.ImagePolicyReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Registry: &imagepolicy.Registry{},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to create controller", "controller", "imagepolicy")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up ready check")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()
	shutdownTracing, err := ops.SetupTracing(ctx, ctrl.Log.WithName("tracing"))
	if err != nil {
		setupLog.Error(err, "Failed to set up tracing")
		os.Exit(1)
	}
	setupLog.Info("Starting manager", "version", version.Version)
	runErr := mgr.Start(ctx)
	// The manager's context is done by now; give the exporter its own
	// deadline to flush the last spans.
	flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdownTracing(flushCtx); err != nil {
		setupLog.Error(err, "Failed to flush traces")
	}
	if runErr != nil {
		setupLog.Error(runErr, "Failed to run manager")
		os.Exit(1)
	}
}
