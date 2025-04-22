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

package infrastructure

import (
	"context"
	controlplanev1beta2 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/controlplane/v1beta2"
	infrastructurev1beta2 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/infrastructure/v1beta2"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// mapAliyunControlPlaneToCluster 在 aliyunControlPlane 的 endpoint 更新时, 及时触发到 aliyunCluster 对象.
func (r *AliyunManagedClusterReconciler) mapAliyunControlPlaneToCluster() handler.MapFunc {
	return func(ctx context.Context, o client.Object) (req []ctrl.Request) {
		log := ctrl.LoggerFrom(ctx)

		aliyunControlPlane, ok := o.(*controlplanev1beta2.AliyunManagedControlPlane)
		if !ok {
			log.Error(errors.Errorf("expected an AliyunManagedControlPlane, got %T instead", o), "")
			return
		}

		log = log.WithValues("aliyunControlPlane", aliyunControlPlane.Name)

		if !aliyunControlPlane.ObjectMeta.DeletionTimestamp.IsZero() {
			return
		}
		if aliyunControlPlane.Spec.ControlPlaneEndpoint.IsZero() {
			return
		}

		cluster, err := util.GetOwnerCluster(ctx, r.Client, aliyunControlPlane.ObjectMeta)
		if err != nil {
			log.Error(err, "failed to get owner Cluster")
			return
		}
		if cluster == nil {
			log.Info("no owner Cluster, skipping mapping")
			return
		}

		// 单纯判断 Kind 不太严谨, 使用 gvk 判断.
		// managedClusterRef := cluster.Spec.InfrastructureRef
		// if managedClusterRef == nil || managedClusterRef.Kind != "AliyunManagedCluster" {
		// 	return
		// }

		gvk, err := apiutil.GVKForObject(
			new(infrastructurev1beta2.AliyunManagedCluster), r.Scheme,
		)
		if err != nil {
			log.Error(err, "failed to find GVK for AliyunManagedCluster")
			return
		}
		gk := gvk.GroupKind()

		// 确保 machinePool 通过 InfrastructureRef 指定的是 aliyunPool 类型
		infraRef := cluster.Spec.InfrastructureRef
		if gk != infraRef.GroupVersionKind().GroupKind() {
			return
		}

		return []ctrl.Request{
			{
				NamespacedName: types.NamespacedName{
					Name:      infraRef.Name,
					Namespace: infraRef.Namespace,
				},
			},
		}
	}
}
