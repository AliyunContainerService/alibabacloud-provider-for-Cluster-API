/*
Copyright 2021 Upbound Inc.
*/

package providerconfig

import (
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/providerconfig"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/upjet/pkg/controller"
	ctrl "sigs.k8s.io/controller-runtime"

	"cluster-api-provider-aliyun/api/alibabacloud/v1beta1"
)

// 手动添加 ProviderConfig, ProviderConfigUsage 的权限列表
//+kubebuilder:rbac:groups=alibabacloud.alibabacloud.com,resources=providerconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=alibabacloud.alibabacloud.com,resources=providerconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=alibabacloud.alibabacloud.com,resources=providerconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=alibabacloud.alibabacloud.com,resources=providerconfigusages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=alibabacloud.alibabacloud.com,resources=providerconfigusages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=alibabacloud.alibabacloud.com,resources=providerconfigusages/finalizers,verbs=update

// Setup adds a controller that reconciles ProviderConfigs by accounting for
// their current usage.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := providerconfig.ControllerName(v1beta1.ProviderConfigGroupKind)

	of := resource.ProviderConfigKinds{
		Config:    v1beta1.ProviderConfigGroupVersionKind,
		UsageList: v1beta1.ProviderConfigUsageListGroupVersionKind,
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1beta1.ProviderConfig{}).
		Watches(&v1beta1.ProviderConfigUsage{}, &resource.EnqueueRequestForProviderConfig{}).
		Complete(providerconfig.NewReconciler(mgr, of,
			providerconfig.WithLogger(o.Logger.WithValues("controller", name)),
			providerconfig.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name)))))
}
