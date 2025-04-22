/*
Copyright (c) 2024-2025, Alibaba Cloud and its affiliates;

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
	"flag"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	xpcontroller "github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/feature"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	tjcontroller "github.com/crossplane/upjet/pkg/controller"
	"github.com/crossplane/upjet/pkg/terraform"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expclusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	alibabacloudv1alpha1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/alibabacloud/v1alpha1"
	alibabacloudv1beta1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/alibabacloud/v1beta1"
	csv1alpha1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/cs/v1alpha1"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/clients"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/config"
	kubernetesnodepoolcontroller "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/controller/cs/kubernetesnodepool"
	managedkubernetescontroller "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/controller/cs/managedkubernetes"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/controller/cs/vpc"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/controller/cs/vswitch"
	providerconfigcontroller "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/controller/providerconfig"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/features"

	controlplanev1beta2 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/controlplane/v1beta2"
	infrastructurev1beta2 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/infrastructure/v1beta2"
	controlplanecontroller "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/controller/controlplane"
	infrastructurecontroller "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/controller/infrastructure"
	//+kubebuilder:scaffold:imports
)

var (
	scheme      = runtime.NewScheme()
	setupLog    = ctrl.Log.WithName("setup")
	providerLog logging.Logger
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(infrastructurev1beta2.AddToScheme(scheme))
	utilruntime.Must(controlplanev1beta2.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	utilruntime.Must(csv1alpha1.AddToScheme(scheme))
	utilruntime.Must(alibabacloudv1alpha1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(alibabacloudv1beta1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(expclusterv1.AddToScheme(scheme))
}

func main() {
	var (
		// kubebuilder
		metricsAddr             string
		enableLeaderElection    bool
		leaderElectionNamespace string
		probeAddr               string
		secureMetrics           bool
		enableHTTP2             bool
		deleteTimeout           time.Duration
		// upjet
		CredentialNamespace string
		CredentialName      string
		CredentialKey       string

		debug                      bool
		pollInterval               time.Duration
		maxReconcileRate           int
		terraformVersion           string
		providerSource             string
		providerVersion            string
		namespace                  string
		enableExternalSecretStores bool
		enableManagementPolicies   bool
	)
	// kubebuilder
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "cluster-api-provider-aliyun-system",
		"The lease resource namespace.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.DurationVar(&deleteTimeout, "delete-timeout", time.Second*300, "The timeout for deleting vpc/vswitch")
	// upjet
	flag.StringVar(&CredentialNamespace, "credential-namespace", "cluster-api-provider-aliyun-system", "the secret's namespace used by ProviderConfig")
	flag.StringVar(&CredentialName, "credential-name", "cluster-api-provider-aliyun-aliyun-tf-creds", "the secret's name used by ProviderConfig")
	flag.StringVar(&CredentialKey, "credential-key", "credentials", "the secret's data field used by ProviderConfig")

	flag.BoolVar(&debug, "debug", false, "Run with debug logging.")
	flag.DurationVar(&pollInterval, "poll", time.Minute*10, "Poll interval controls how often an individual resource should be checked for drift.")
	flag.IntVar(&maxReconcileRate, "max-reconcile-rate", 10, "The global maximum rate per second at which resources may be checked for drift from the desired state.")

	flag.StringVar(&terraformVersion, "terraform-version", "1.2.1", "Terraform version.")
	flag.StringVar(&providerSource, "terraform-provider-source", "aliyun/alicloud", "Terraform provider source.")
	flag.StringVar(&providerVersion, "terraform-provider-version", "1.223.2", "Terraform provider version.")

	flag.StringVar(&namespace, "namespace", "crossplane-system", "Namespace used to set as default scope in default secret store config.")
	flag.BoolVar(&enableExternalSecretStores, "enable-external-secret-stores", false,
		"Enable support for ExternalSecretStores.")
	flag.BoolVar(&enableManagementPolicies, "enable-management-policies", true,
		"Enable support for Management Policies.")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	providerLog = logging.NewLogrLogger(zap.New(zap.UseFlagOptions(&opts)).WithName("provider-alibabacloud"))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	err := clients.EssEndpointList.Init()
	if err != nil {
		setupLog.Error(err, "unable to init ess endpoint list")
		os.Exit(1)
	}
	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})
	ctx := ctrl.SetupSignalHandler()
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			SyncPeriod: &pollInterval,
		},
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:           webhookServer,
		HealthProbeBindAddress:  probeAddr,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "a06eab04.cluster.x-k8s.io",
		LeaderElectionNamespace: leaderElectionNamespace,
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
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&infrastructurecontroller.AliyunManagedClusterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AliyunManagedCluster")
		os.Exit(1)
	}

	kubeClient, err := kubernetes.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		setupLog.Error(err, "create kube client failed")
		os.Exit(1)
	}
	secret, err := kubeClient.CoreV1().Secrets(CredentialNamespace).Get(ctx, CredentialName, metav1.GetOptions{})
	if err != nil {
		// if apierrors.IsNotFound(err) {
		// }
		setupLog.Error(
			err, "failed to get credential secret",
			"namespace", CredentialNamespace, "name", CredentialName,
		)
		os.Exit(1)
	}
	credentialSecret := &controlplanecontroller.CredentialSecret{
		Namespace: CredentialNamespace,
		Name:      CredentialName,
		Key:       CredentialKey,
	}
	if err = credentialSecret.Decode(secret); err != nil {
		setupLog.Error(
			err, "credential secret parse failed",
			"namespace", CredentialNamespace, "name", CredentialName,
		)
		os.Exit(1)
	}
	if err = (&controlplanecontroller.AliyunManagedControlPlaneReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		CredentialSecret: credentialSecret,
		DeleteTimeout:    deleteTimeout,
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AliyunManagedControlPlane")
		os.Exit(1)
	}
	if err = (&infrastructurecontroller.AliyunManagedMachinePoolReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		DeleteTimeout: deleteTimeout,
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AliyunManagedMachinePool")
		os.Exit(1)
	}
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = (&controlplanev1beta2.AliyunManagedControlPlane{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AliyunManagedControlPlane")
			os.Exit(1)
		}
	}
	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = (&infrastructurev1beta2.AliyunManagedMachinePool{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AliyunManagedMachinePool")
			os.Exit(1)
		}
	}
	//+kubebuilder:scaffold:builder

	// upjet
	o := tjcontroller.Options{
		Options: xpcontroller.Options{
			Logger:                  providerLog,
			GlobalRateLimiter:       ratelimiter.NewGlobal(maxReconcileRate),
			PollInterval:            pollInterval,
			MaxConcurrentReconciles: maxReconcileRate,
			Features:                &feature.Flags{},
		},
		Provider: config.GetProvider(),
		// use the following WorkspaceStoreOption to enable the shared gRPC mode
		// terraform.WithProviderRunner(terraform.NewSharedProvider(log, os.Getenv("TERRAFORM_NATIVE_PROVIDER_PATH"), terraform.WithNativeProviderArgs("-debuggable")))
		WorkspaceStore: terraform.NewWorkspaceStore(providerLog),
		SetupFn:        clients.TerraformSetupBuilder(terraformVersion, providerSource, providerVersion),
	}

	if enableExternalSecretStores {
		o.SecretStoreConfigGVK = &alibabacloudv1alpha1.StoreConfigGroupVersionKind
		providerLog.Info("Alpha feature enabled", "flag", features.EnableAlphaExternalSecretStores)

		// Ensure default store config exists.
		// kingpin.FatalIfError(resource.Ignore(kerrors.IsAlreadyExists, ), "cannot create default store config")
		if err := mgr.GetClient().Create(context.Background(), &alibabacloudv1alpha1.StoreConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
			Spec: alibabacloudv1alpha1.StoreConfigSpec{
				// NOTE(turkenh): We only set required spec and expect optional
				// ones to properly be initialized with CRD level default values.
				SecretStoreConfig: xpv1.SecretStoreConfig{
					DefaultScope: namespace,
				},
			},
		}); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				providerLog.Info("cannot create default store config", "error", err)
				os.Exit(1)
			}
		}
	}

	if enableManagementPolicies {
		o.Features.Enable(features.EnableBetaManagementPolicies)
		providerLog.Info("Beta feature enabled", "flag", features.EnableBetaManagementPolicies)
	}

	if err := managedkubernetescontroller.Setup(mgr, o); err != nil {
		providerLog.Info("failed to setup managedkubernetes controller", "error", err)
		os.Exit(1)
	}
	if err := kubernetesnodepoolcontroller.Setup(mgr, o); err != nil {
		providerLog.Info("failed to setup kubernetesnodepool controller", "error", err)
		os.Exit(1)
	}
	// upjet vpc controler 负责与阿里云对接, 完成创建、更新、删除等同步功能.
	if err := vpc.Setup(mgr, o); err != nil {
		providerLog.Info("failed to setup kubernetesnodepool controller", "error", err)
		os.Exit(1)
	}
	// vpc reconciler 负责残留的 vpc 资源的清理工作, 监听的也是 VPC{}
	if err = (&vpc.VPCReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		DeleteTimeout: deleteTimeout,
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VPC")
		os.Exit(1)
	}
	// 同 upjet vpc controler
	if err := vswitch.Setup(mgr, o); err != nil {
		providerLog.Info("failed to setup kubernetesnodepool controller", "error", err)
		os.Exit(1)
	}
	// 同 vpc reconciler
	if err = (&vswitch.VswitchReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		DeleteTimeout: deleteTimeout,
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Vswitch")
		os.Exit(1)
	}
	if err := providerconfigcontroller.Setup(mgr, o); err != nil {
		providerLog.Info("failed to setup providerconfig controller", "error", err)
		os.Exit(1)
	}

	// upjet end

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
