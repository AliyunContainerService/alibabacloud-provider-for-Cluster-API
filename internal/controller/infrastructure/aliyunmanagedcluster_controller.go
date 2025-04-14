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

package infrastructure

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	controlplanev1beta2 "cluster-api-provider-aliyun/api/controlplane/v1beta2"
	infrastructurev1beta2 "cluster-api-provider-aliyun/api/infrastructure/v1beta2"
)

// AliyunManagedClusterReconciler reconciles a AliyunManagedCluster object
type AliyunManagedClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// 手动添加 Cluster 的权限列表
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters/finalizers,verbs=update

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmanagedclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmanagedclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmanagedclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AliyunManagedCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *AliyunManagedClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(5).Info("Reconcile AliyunManagedCluster")

	// TODO(user): your logic here
	// Fetch the AliyunManagedCluster instance
	aliyunManagedCluster := &infrastructurev1beta2.AliyunManagedCluster{}
	err := r.Get(ctx, req.NamespacedName, aliyunManagedCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, aliyunManagedCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	// todo: annotations.IsPaused()

	log = log.WithValues("cluster", cluster.Name)

	controlPlane := &controlplanev1beta2.AliyunManagedControlPlane{}
	controlPlaneRef := types.NamespacedName{
		Name:      cluster.Spec.ControlPlaneRef.Name,
		Namespace: cluster.Spec.ControlPlaneRef.Namespace,
	}

	if err := r.Get(ctx, controlPlaneRef, controlPlane); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get control plane ref: %w", err)
	}

	log = log.WithValues("controlPlane", controlPlaneRef.Name)

	// Set the values from the managed control plane
	// 为 .spec.controlPlaneEndpoint 赋值, 是 cluster 资源变成 Provisioned 状态的判断条件
	aliyunManagedCluster.Spec.ControlPlaneEndpoint = controlPlane.Spec.ControlPlaneEndpoint
	err = r.Client.Update(ctx, aliyunManagedCluster, &client.UpdateOptions{})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update aliyunManagedCluster: %w", err)
	}
	// .status.ready 是 cluster-api 中 reconcile 的一个步骤
	aliyunManagedCluster.Status.Ready = true
	aliyunManagedCluster.Status.FailureDomains = controlPlane.Status.FailureDomains
	err = r.Client.Status().Update(ctx, aliyunManagedCluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update aliyunManagedCluster: %w", err)
	}
	log.V(5).Info("Successfully reconciled AliyunManagedCluster")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AliyunManagedClusterReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) (err error) {
	// log := ctrl.LoggerFrom(ctx)

	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1beta2.AliyunManagedCluster{}).
		// Complete(r)
		Build(r)
	if err != nil {
		return
	}

	// control plane endpoint 字段检测
	if err = controller.Watch(
		source.Kind(mgr.GetCache(), &controlplanev1beta2.AliyunManagedControlPlane{}),
		handler.EnqueueRequestsFromMapFunc(
			r.mapAliyunControlPlaneToCluster(),
		),
	); err != nil {
		return
	}

	return
}
