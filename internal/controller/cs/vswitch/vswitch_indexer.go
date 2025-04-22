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

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/cs/v1alpha1"
	machinepoolv1beta2 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/infrastructure/v1beta2"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/common"
)

// indexVSwitchByAliyunPoolUID Indexer 索引器, 通过 aliyunPool uid 找到其子级的 vswitch 资源.
func (r *VswitchReconciler) indexVSwitchByAliyunPoolUID(rawObj client.Object) []string {
	vswitch := rawObj.(*v1alpha1.Vswitch)
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

// Reconcile 只做删除超时的强制清理工作, 防止资源残留.
func (r *VswitchReconciler) aliyunMachinePoolToVSwitchMapFunc() handler.MapFunc {
	return func(ctx context.Context, obj client.Object) (req []ctrl.Request) {
		log := ctrl.LoggerFrom(ctx)

		aliyunPool, ok := obj.(*machinepoolv1beta2.AliyunManagedMachinePool)
		if !ok {
			log.Error(errors.Errorf("expected an AliyunManagedMachinePool, got %T instead", obj), "")
			return
		}

		// 获取属于当前 aliyunPool 的 vswitch 列表
		var vswitches v1alpha1.VswitchList
		listOpts := client.MatchingFields{
			common.IndexVSwitchByAliyunPoolUID: string(aliyunPool.UID),
		}
		if err := r.List(ctx, &vswitches, listOpts); err != nil {
			log.Error(
				err, "unable to get vswitches of AliyunManagedMachinePool",
				"uid", aliyunPool.UID, "AliyunManagedMachinePool", aliyunPool.Name,
			)
			return
		}
		for _, item := range vswitches.Items {
			req = append(req, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.Name,
					Namespace: item.Namespace,
				},
			})
		}
		return
	}
}
