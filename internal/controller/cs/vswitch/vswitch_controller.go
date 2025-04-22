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

package vswitch

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/cs/v1alpha1"
	csv1alpha1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/cs/v1alpha1"
	machinepoolv1beta2 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/infrastructure/v1beta2"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/common"
)

const (
	Finalizer = "finalizer.managedresource.crossplane.io"
)

type VswitchReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	DeleteTimeout time.Duration // DeleteTimeout 超时时间
}

// Reconcile 只做删除超时的强制清理工作, 防止资源残留.
func (r *VswitchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrl.LoggerFrom(ctx).WithValues("vswitch", req.Name)

	vswitch := &v1alpha1.Vswitch{}
	if err = r.Get(ctx, req.NamespacedName, vswitch); err != nil && !apierrors.IsNotFound(err) {
		return
	}

	if !vswitch.DeletionTimestamp.IsZero() {
		// 判断超时
		metav1Time := metav1.NewTime(time.Now().Add(-r.DeleteTimeout))
		if vswitch.ObjectMeta.DeletionTimestamp.Before(&metav1Time) {
			log.Info("delete timeout, try to delete it forcely")
			controllerutil.RemoveFinalizer(vswitch, Finalizer)
			if err = r.Client.Update(ctx, vswitch, &client.UpdateOptions{}); err != nil {
				return
			}
			log.Info("delete forcely success")
		}
		return
	}

	ownerRefs := vswitch.GetOwnerReferences()
	if len(ownerRefs) == 0 {
		// todo: 没有属主, 暂时不清理, 有可能是手动创建的.
		log.Info("there is no OwnerReferences in vswitch, ignore")
		return
	}
	ownerRef := ownerRefs[0]
	if ownerRef.Kind != "AliyunManagedMachinePool" {
		return
	}

	////////////////////////////////////////////////////////////////////////////
	// 1. 在属主 aliyunPool 中查找是否存在引用.

	aliyunPoolList := machinepoolv1beta2.AliyunManagedMachinePoolList{}
	listOpts := client.MatchingFields{
		common.IndexAliyunPoolByUID: string(ownerRef.UID),
	}
	if err = r.List(ctx, &aliyunPoolList, listOpts); err != nil {
		log.Error(err,
			"unable to get AliyunManagedMachinePoolList by index",
			"uid", ownerRef.UID,
		)
		return
	}
	if len(aliyunPoolList.Items) == 0 {
		// 属主不在了, 则可以清理了
		err = r.Client.Delete(ctx, vswitch)
		if err != nil {
			return
		}
		return
	}
	aliyunPool := aliyunPoolList.Items[0]

	for _, vsw := range aliyunPool.Spec.ScalingGroup.VSwitches {
		if vsw.ID != "" {
			// 只关注自建的 vswitch
			continue
		}
		if vsw.UID == string(vswitch.UID) {
			// 新增 vsw.UID 字段, 就算 vswitch 是创建中, 也可以建立引用关系.
			return
		}
	}

	////////////////////////////////////////////////////////////////////////////
	// 2. 在属主 aliyunPool 名下的 nodepool 中查找是否存在引用.
	//
	// 原因在于, 如果 aliyunPool 变更了 vswitch 信息, 就立刻清理的话, 可能会导致
	// nodepool 在变更后同步的时候报错: InvalidVSwitchId.NotFoundSpecifiedVSwitch
	// (terraform 需要原状态是正常的, 才能正常更新)
	// 因此要确保当前 vswitch 在 aliyunPool 与 nodepool 中都不存在引用才可以清理.

	nodepoolList := csv1alpha1.KubernetesNodePoolList{}
	nodepoolListOpts := client.MatchingFields{
		common.IndexNodePoolByAliyunPoolUID: string(aliyunPool.UID),
	}
	if err = r.List(ctx, &nodepoolList, nodepoolListOpts); err != nil {
		log.Error(err,
			"unable to get KubernetesNodePoolList by index",
			"uid", ownerRef.UID,
		)
		return
	}
	if len(nodepoolList.Items) == 0 {
		// 属主不在了, 则可以清理了
		err = r.Client.Delete(ctx, vswitch)
		return
	}
	nodepool := nodepoolList.Items[0]

	if len(nodepool.Status.AtProvider.VswitchIds) == 0 {
		err = r.Client.Delete(ctx, vswitch)
		return
	}

	for _, id := range nodepool.Status.AtProvider.VswitchIds {
		if vswitch.Status.AtProvider.ID == nil {
			continue
		}
		if *vswitch.Status.AtProvider.ID == *id {
			return
		}
	}

	////////////////////////////////////////////////////////////////////////////

	// 如果属主中已没有当前 vswitch 的记录, 表示其已经被替换, 需要清理.
	if err = r.Client.Delete(ctx, vswitch); err != nil {
		return
	}
	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *VswitchReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	// 添加 Indexer 索引器, 可以通过 aliyunPool uid 找到其子级的 vswitch 资源.
	if err := mgr.GetFieldIndexer().IndexField(
		ctx, &v1alpha1.Vswitch{}, common.IndexVSwitchByAliyunPoolUID,
		r.indexVSwitchByAliyunPoolUID,
	); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Vswitch{}).
		// 屏蔽所有 Add 事件.
		// 因为 VswitchReconciler{} 只做清理工作, 对于 Add 事件不应被触发.
		//
		// AliyunManagedControlPlane/AliyunManagedMachinePool 在处理
		// vSwitches[] 列表时, 会先赋值 UID 字段建立联系, 以免 VswitchReconciler{}
		// 误将创建中的 vswitch 删除(毕竟创建起始时 resourceID 字段还为空)
		//
		// 最重要的是, alicp/alimp 从赋值 UID 字段到实际 Update() 需要经历的时间较长,
		// 而 vswitch 在创建时就立刻进入 VswitchReconciler{} 主流程, 两者存在时间差,
		// 而`vsw.UID == string(vswitch.UID)`的判断不成立,
		// 会导致新建的 vswitch 立刻被删除, 陷入"创建-删除"的死循环.
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(event.CreateEvent) bool {
				return false
			},
		}).
		// Watches(
		// 	// 监测 aliyunPool 的变动事件, 如果 vswitch 列表发生, 需要及时通过所属的 vswitch
		// 	&machinepoolv1beta2.AliyunManagedMachinePool{},
		// 	handler.EnqueueRequestsFromMapFunc(
		// 		r.aliyunMachinePoolToVSwitchMapFunc(),
		// 	),
		// ).
		Complete(r)
}
