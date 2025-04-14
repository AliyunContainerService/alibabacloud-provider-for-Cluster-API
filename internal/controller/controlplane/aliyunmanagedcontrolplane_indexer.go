/*
*Copyright (c) 2024-2025, Alibaba Cloud and its affiliates;
*Licensed under the Apache License, Version 2.0 (the "License");
*you may not use this file except in compliance with the License.
*You may obtain a copy of the License at

*   http://www.apache.org/licenses/LICENSE-2.0

*Unless required by applicable law or agreed to in writing, software
*distributed under the License is distributed on an "AS IS" BASIS,
*WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
*See the License for the specific language governing permissions and
*limitations under the License.
 */

package controlplane

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	controlplanev1beta2 "cluster-api-provider-aliyun/api/controlplane/v1beta2"
	csv1alpha1 "cluster-api-provider-aliyun/api/cs/v1alpha1"
	"cluster-api-provider-aliyun/internal/common"
)

func (r *AliyunManagedControlPlaneReconciler) indexAliyunControlPlaneByUID(rawObj client.Object) []string {
	controlPlane := rawObj.(*controlplanev1beta2.AliyunManagedControlPlane)
	return []string{string(controlPlane.UID)}
}

// mapManagedKubernetesToAliyunControlPlane 在 ManagedKubernetes 状态更新时, 及时触发到 aliyunControlPlane 对象.
func (r *AliyunManagedControlPlaneReconciler) mapManagedKubernetesToAliyunControlPlane(_ context.Context) handler.MapFunc {
	return func(ctx context.Context, o client.Object) (req []ctrl.Request) {
		log := ctrl.LoggerFrom(ctx)

		managedKubernetes, ok := o.(*csv1alpha1.ManagedKubernetes)
		if !ok {
			log.Error(errors.Errorf("expected an ManagedKubernetes, got %T instead", o), "")
			return
		}
		log = log.WithValues("ManagedKubernetes", managedKubernetes.Name)

		ownerRefs := managedKubernetes.GetOwnerReferences()
		if len(ownerRefs) == 0 {
			log.Error(errors.Errorf("failed to get OwnerReferences of ManagedKubernetes"), "")
			return
		}
		ownerRef := ownerRefs[0]

		// 查询索引器, 使用 aliyunControlPlane 的 uid 查询对应的资源
		// 没有传入 namespace 选项, 因此会在所有 ns 中查询符合条件的资源.
		var aliyunControlPlaneList controlplanev1beta2.AliyunManagedControlPlaneList
		listOpts := client.MatchingFields{
			common.IndexAliyunControlPlaneByUID: string(ownerRef.UID),
		}
		if err := r.List(ctx, &aliyunControlPlaneList, listOpts); err != nil {
			log.Error(
				err, "unable to get AliyunManagedControlPlaneList by index",
				"uid", ownerRef.UID,
			)
			return
		}
		// 通知 managedKubernetes 的 属主 control plane.
		// 一般(不出意外的话) uid 不会存在重复, 所以这里 items 只会有一个成员.
		for _, item := range aliyunControlPlaneList.Items {
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
