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

package vpc

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"cluster-api-provider-aliyun/api/cs/v1alpha1"
)

const (
	Finalizer = "finalizer.managedresource.crossplane.io"
)

type VPCReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	DeleteTimeout time.Duration // DeleteTimeout 超时时间
}

// Reconcile 只做删除超时的强制清理工作, 防止资源残留.
func (r *VPCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("vpc", req.Name)

	vpc := &v1alpha1.VPC{}

	if err := r.Get(ctx, req.NamespacedName, vpc); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get vpc: %w", err)
	}

	// 没有被删除, 直接忽略
	if vpc.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	// 判断超时
	metav1Time := metav1.NewTime(time.Now().Add(-r.DeleteTimeout))
	if vpc.ObjectMeta.DeletionTimestamp.Before(&metav1Time) {
		log.Info("delete timeout, try to delete it forcely")
		controllerutil.RemoveFinalizer(vpc, Finalizer)
		if err := r.Client.Update(ctx, vpc, &client.UpdateOptions{}); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("delete forcely success")
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VPCReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.VPC{}).
		// 屏蔽所有 Add 事件(具体原因见 vswitch controller)
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(event.CreateEvent) bool {
				return false
			},
		}).
		Complete(r)
}
