/*
Copyright 2024.

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

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expclusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	controlplanev1beta2 "cluster-api-provider-aliyun/api/controlplane/v1beta2"
	csv1alpha1 "cluster-api-provider-aliyun/api/cs/v1alpha1"
	machinepoolv1beta2 "cluster-api-provider-aliyun/api/infrastructure/v1beta2"
	"cluster-api-provider-aliyun/internal/common"
)

// indexAliyunPoolByUID Indexer 索引器, 通过 uid 信息找到 aliyunPool 资源.
// 用于子资源通过 ownerReferences[].uid 找到父级资源.
func (r *AliyunManagedMachinePoolReconciler) indexAliyunPoolByUID(rawObj client.Object) []string {
	aliyunPool := rawObj.(*machinepoolv1beta2.AliyunManagedMachinePool)
	return []string{string(aliyunPool.UID)}
}

// indexNodePoolByAliyunPoolUID Indexer 索引器, 通过 aliyunPool uid 找到其子级的 KubernetesNodePool 资源.
func (r *AliyunManagedMachinePoolReconciler) indexNodePoolByAliyunPoolUID(rawObj client.Object) []string {
	vswitch := rawObj.(*csv1alpha1.KubernetesNodePool)
	ownerRefs := vswitch.GetOwnerReferences()
	if len(ownerRefs) == 0 {
		// todo: 没有属主, 有可能是手动创建的.
		return []string{}
	}
	ownerRef := ownerRefs[0]
	if ownerRef.Kind != "AliyunManagedMachinePool" {
		return []string{}
	}
	return []string{string(ownerRef.UID)}
}

// mapMachinePoolToAliyunPool 监听 machine pool 副本数变更, 加快响应
func (r *AliyunManagedMachinePoolReconciler) mapMachinePoolToAliyunPool() handler.MapFunc {
	return func(ctx context.Context, o client.Object) (req []ctrl.Request) {
		log := ctrl.LoggerFrom(ctx)

		machinePool, ok := o.(*expclusterv1.MachinePool)
		if !ok {
			log.Error(nil, "Expected a MachinePool but got a %T", o)
		}

		if !machinePool.ObjectMeta.DeletionTimestamp.IsZero() {
			return
		}

		gvk, err := apiutil.GVKForObject(
			new(machinepoolv1beta2.AliyunManagedMachinePool), r.Scheme,
		)
		if err != nil {
			log.Error(err, "failed to find GVK for AliyunManagedMachinePool")
			return
		}
		gk := gvk.GroupKind()

		// 确保 machinePool 通过 InfrastructureRef 指定的是 aliyunPool 类型
		infraGK := machinePool.Spec.Template.Spec.InfrastructureRef.GroupVersionKind().GroupKind()
		if gk != infraGK {
			return
		}

		return []ctrl.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: machinePool.Namespace,
					Name:      machinePool.Spec.Template.Spec.InfrastructureRef.Name,
				},
			},
		}
	}
}

// mapAliyunControlPlaneToAliyunPool aliyunControlPlane ready 后, aliyunPool 才会
// 开始真正的创建流程, 这里在 aliyunControlPlane 发生变动后及时通知到其名下的 aliyunPool 资源.
func (r *AliyunManagedMachinePoolReconciler) mapAliyunControlPlaneToAliyunPool() handler.MapFunc {
	return func(ctx context.Context, o client.Object) (req []ctrl.Request) {
		log := ctrl.LoggerFrom(ctx)

		aliyunControlPlane, ok := o.(*controlplanev1beta2.AliyunManagedControlPlane)
		if !ok {
			log.Error(nil, "Expected a AliyunManagedControlPlane but got a %T", o)
		}

		if !aliyunControlPlane.ObjectMeta.DeletionTimestamp.IsZero() {
			return
		}

		// aliyunPool 的属主并不直接是 aliyunControlPlane, 需要通过 cluster 资源定位.
		//
		// Cluster.spec.controlPlaneRef 使用名称与 aliyunControlPlane 建立联系,
		// 借鉴自 cluster-api-provider-aws v2.4.0, 符合 cluster-api 规范,
		// 因此这里不再建立 uid 索引器.
		clusterKey, err := getOwnerClusterKey(aliyunControlPlane.ObjectMeta)
		if err != nil {
			log.Error(err, "couldn't get AliyunManagedControlPlane owner ObjectKey")
			return
		}
		if clusterKey == nil {
			return
		}

		// control plane ready 的信息需要通知到其名下所有 machine pool 资源.
		machinePoolList := expclusterv1.MachinePoolList{}
		listOpts := client.MatchingLabels{
			clusterv1.ClusterNameLabel: clusterKey.Name,
		}
		if err := r.Client.List(
			ctx, &machinePoolList, client.InNamespace(clusterKey.Namespace), listOpts,
		); err != nil {
			log.Error(err, "couldn't list machine pools for cluster")
			return
		}

		// 上面已经有 machinePool -> aliyunPool 的映射函数, 复用即可.
		mapFunc := r.mapMachinePoolToAliyunPool()

		for _, machinePool := range machinePoolList.Items {
			mp := machinePool
			aliyunPool := mapFunc(ctx, &mp)
			req = append(req, aliyunPool...)
		}

		return
	}
}

// mapNodePoolToAliyunPool 在 nodepool 的状态更新时, 及时触发到 aliyunPool 属主对象.
func (r *AliyunManagedMachinePoolReconciler) mapNodePoolToAliyunPool() handler.MapFunc {
	return func(ctx context.Context, o client.Object) (req []ctrl.Request) {
		log := ctrl.LoggerFrom(ctx)

		nodepool, ok := o.(*csv1alpha1.KubernetesNodePool)
		if !ok {
			log.Error(errors.Errorf("expected an KubernetesNodePool, got %T instead", o), "")
			return
		}
		log = log.WithValues("KubernetesNodePool", nodepool.Name)

		ownerRefs := nodepool.GetOwnerReferences()
		if len(ownerRefs) == 0 {
			log.Error(errors.Errorf("failed to get OwnerReferences of KubernetesNodePool"), "")
			return
		}
		ownerRef := ownerRefs[0]

		// 查询索引器, 使用 aliyunPool 的 uid 查询对应的资源
		// 没有传入 namespace 选项, 因此会在所有 ns 中查询符合条件的资源.
		var aliyunPoolList machinepoolv1beta2.AliyunManagedMachinePoolList
		listOpts := client.MatchingFields{
			common.IndexAliyunPoolByUID: string(ownerRef.UID),
		}
		if err := r.List(ctx, &aliyunPoolList, listOpts); err != nil {
			log.Error(
				err, "unable to get AliyunManagedMachinePoolList by index",
				"uid", ownerRef.UID,
			)
			return
		}
		// 通知 nodepool 的属主 aliyunPool.
		// 一般(不出意外的话) uid 不会存在重复, 所以这里 items 只会有一个成员.
		for _, item := range aliyunPoolList.Items {
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.Name,
					Namespace: item.Namespace,
				},
			})
		}
		return req
	}
}
