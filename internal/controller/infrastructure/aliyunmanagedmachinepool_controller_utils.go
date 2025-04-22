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
	"fmt"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expclusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	csv1alpha1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/cs/v1alpha1"
	machinepoolv1beta2 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/infrastructure/v1beta2"
)

// getOwnerMachinePool 查询目标资源 obj (aliyunPool)的 MachinePool 属主并返回.
//
//	@param obj: aliyunPool.ObjectMeta 成员字段
//
// getOwnerMachinePool returns the MachinePool object owning the current resource.
func getOwnerMachinePool(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*expclusterv1.MachinePool, error) {
	for _, ref := range obj.OwnerReferences {
		if ref.Kind != "MachinePool" {
			continue
		}
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if gv.Group == expclusterv1.GroupVersion.Group {
			return getMachinePoolByName(ctx, c, obj.Namespace, ref.Name)
		}
	}
	return nil, nil
}

// getMachinePoolByName finds and return a Machine object using the specified params.
func getMachinePoolByName(ctx context.Context, c client.Client, namespace, name string) (*expclusterv1.MachinePool, error) {
	m := &expclusterv1.MachinePool{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, m); err != nil {
		return nil, err
	}
	return m, nil
}

// getOwnerClusterKey 查询目标资源 obj (aliyunControlPlane)的 Cluster 属主并返回.
//
//	@param obj: aliyunControlPlane.ObjectMeta 成员字段
//
// getOwnerClusterKey returns only the Cluster name and namespace.
func getOwnerClusterKey(obj metav1.ObjectMeta) (*client.ObjectKey, error) {
	for _, ref := range obj.OwnerReferences {
		if ref.Kind != "Cluster" {
			continue
		}
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if gv.Group == clusterv1.GroupVersion.Group {
			return &client.ObjectKey{
				Namespace: obj.Namespace,
				Name:      ref.Name,
			}, nil
		}
	}
	return nil, nil
}

const PasswordSecretKey = "password"

func getPasswordSecretName(poolName string) string {
	return fmt.Sprintf("%s-password", poolName)
}

func (r *AliyunManagedMachinePoolReconciler) ensurePasswordAndKeyName(
	ctx context.Context,
	aliyunPool *machinepoolv1beta2.AliyunManagedMachinePool,
	kubernetesNodePool *csv1alpha1.KubernetesNodePool,
) (err error) {
	if aliyunPool.Spec.ScalingGroup.KeyName != "" {
		kubernetesNodePool.Spec.ForProvider.KeyName = &aliyunPool.Spec.ScalingGroup.KeyName
		kubernetesNodePool.Spec.ForProvider.PasswordSecretRef = nil
	} else if aliyunPool.Spec.ScalingGroup.Password != "" {
		if err := r.ensurePasswordSecret(ctx, aliyunPool); err != nil {
			return fmt.Errorf("ensurePasswordSecret failed: %s", err)
		}
		kubernetesNodePool.Spec.ForProvider.PasswordSecretRef = &xpv1.SecretKeySelector{
			SecretReference: xpv1.SecretReference{
				Namespace: aliyunPool.Namespace,
				Name:      getPasswordSecretName(aliyunPool.Name),
			},
			Key: PasswordSecretKey,
		}
		kubernetesNodePool.Spec.ForProvider.KeyName = nil
	}
	return
}

func (r *AliyunManagedMachinePoolReconciler) ensurePasswordSecret(
	ctx context.Context, aliyunPool *machinepoolv1beta2.AliyunManagedMachinePool,
) (err error) {
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Namespace: aliyunPool.Namespace,
		Name:      getPasswordSecretName(aliyunPool.Name),
	}
	var exist bool

	if err := r.Client.Get(ctx, secretKey, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get ManagedKubernetes failed: %s", err)
		}
	} else {
		exist = true
	}
	if !exist {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretKey.Name,
				Namespace: secretKey.Namespace,
				Labels:    map[string]string{},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         aliyunPool.TypeMeta.GroupVersionKind().GroupVersion().String(),
						Kind:               aliyunPool.TypeMeta.Kind,
						Name:               aliyunPool.Name,
						UID:                aliyunPool.UID,
						Controller:         ptr.To[bool](true),
						BlockOwnerDeletion: ptr.To[bool](true),
					},
				},
			},
			Data: map[string][]byte{
				PasswordSecretKey: []byte(aliyunPool.Spec.ScalingGroup.Password),
			},
		}
		if err = r.Client.Create(ctx, secret, &client.CreateOptions{}); err != nil {
			return err
		}
	} else {
		// 如存在则更新
		secret.Data[PasswordSecretKey] = []byte(aliyunPool.Spec.ScalingGroup.Password)
		if err = r.Client.Update(ctx, secret); err != nil {
			return err
		}
	}

	return
}
