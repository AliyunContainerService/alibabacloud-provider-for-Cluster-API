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

package common

import (
	"context"
	"errors"
	"fmt"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	upjetresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonapi "cluster-api-provider-aliyun/api/common"
	csv1alpha1 "cluster-api-provider-aliyun/api/cs/v1alpha1"
)

var ErrorTimeout error = errors.New("delete timeout")

// ReconcileVSwitch ...
//
// @param object: aliyunManagedControlPlane 或 aliyunManagedMachinePool 对象
// @param keyName: ACK集群名称或NodePool名称
func ReconcileVSwitch(
	ctx context.Context, log logr.Logger,
	kubeclient client.Client, object runtime.Object, keyName string,
	resourceSpec xpv1.ResourceSpec,
	vSwitches *commonapi.VSwitches, vpcID string,
) (vswitchIDs []*string, err error) {
	// 如果指定已有 vswitch 资源, 或是所有自建 vswitch 都已创建成功.
	vswitchIDs, err = vSwitches.GetIDList()
	if err == nil {
		return
	}

	var errList []error
	for i := range *vSwitches {
		// 获取成员指针
		vsw := &((*vSwitches)[i])
		if vsw.ID != "" || (vsw.ResourceID != "" && vsw.UID != "") {
			continue
		}

		// 如果未指定, 则根据 VPC 字段自行创建并记录id
		vswKey := types.NamespacedName{
			Name: vsw.Name,
		}
		vswitch := &csv1alpha1.Vswitch{}

		var exist bool
		if err = kubeclient.Get(ctx, vswKey, vswitch); err != nil {
			if !apierrors.IsNotFound(err) {
				return
			}
			log.WithValues("vswitch", vsw.Name).Info("vswitch not found, try to create")
		} else {
			// (废弃)如果存在同名资源, 但其 cidr 与配置值不符
			// 如果存在同名资源, 但其属主不是自己, 则报错.
			//
			// 阿里云可以创建同名+同网段+同区域的 VPC/VSwitch 资源, ta们的 id 会不一样,
			// 但是 terraform 不行, 在发现同名资源时会报错, 无法创建.
			//
			// todo: 同一集群的 control plane 与 machine pool 也无法使用同名 vswitch.
			ownerRefs := vswitch.GetOwnerReferences()
			if len(ownerRefs) == 0 || ownerRefs[0].UID != object.(metav1.Object).GetUID() {
				err = fmt.Errorf(
					"vswitch is already exist: %s", vsw.Name,
				)
				return
			}
			exist = true
		}

		if exist {
			// 假设 vswitch 正在创建中.
			if vswitch.Status.AtProvider.ID != nil && *vswitch.Status.AtProvider.ID != "" {
				vsw.ResourceID = *vswitch.Status.AtProvider.ID
				vsw.UID = string(vswitch.UID)
				continue
			}
			// 如果出现错误
			// 根据 crossplane 资源状态设置 condition, 但对 control plane 的就绪状态并没有决定性影响
			var syncedCondition = vswitch.GetCondition(xpv1.TypeSynced)
			if syncedCondition.Message != "" {
				errList = append(errList, fmt.Errorf("vswitch synced failed: %s", syncedCondition.Message))
				continue
			}
			var asyncCondition = vswitch.GetCondition(upjetresource.TypeAsyncOperation)
			if asyncCondition.Message != "" {
				errList = append(errList, fmt.Errorf("vswitch async failed: %s", asyncCondition.Message))
				continue
			}
			var lastAsyncCondition = vswitch.GetCondition(upjetresource.TypeLastAsyncOperation)
			if lastAsyncCondition.Message != "" {
				errList = append(errList, fmt.Errorf("vswitch lastAsync failed: %s", lastAsyncCondition.Message))
				continue
			}
			// 如果只是单纯未完成创建
			errList = append(errList, fmt.Errorf("vswitch not ready"))
		} else {
			gvk := object.GetObjectKind().GroupVersionKind()
			description := commonapi.GenerateVSwitchDescription(gvk.Kind, keyName)
			vswitch = &csv1alpha1.Vswitch{
				ObjectMeta: metav1.ObjectMeta{
					Name: vsw.Name,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         gvk.GroupVersion().String(),
							Kind:               gvk.Kind,
							Name:               object.(metav1.Object).GetName(),
							UID:                object.(metav1.Object).GetUID(),
							Controller:         ptr.To[bool](true),
							BlockOwnerDeletion: ptr.To[bool](true),
						},
					},
				},
				Spec: csv1alpha1.VswitchSpec{
					ForProvider: csv1alpha1.VswitchParameters{
						VswitchName: &vsw.Name,
						CidrBlock:   &vsw.CIDRBlock,
						VPCID:       &vpcID,
						ZoneID:      &vsw.ZoneID,
						Description: &description,
					},
					ResourceSpec: resourceSpec,
				},
			}
			if err := kubeclient.Create(ctx, vswitch, &client.CreateOptions{}); err != nil {
				errList = append(errList, fmt.Errorf("vswitch create failed: %s", err))
				continue
			}
			vsw.UID = string(vswitch.UID)
			log.WithValues("vswitch", vsw.Name).Info("vswitch create success")
		}
	}
	if len(errList) != 0 {
		return nil, utilerrors.NewAggregate(errList)
	}
	return vSwitches.GetIDList()
}

func ReconcileRemoveVSwitch(
	ctx context.Context, log logr.Logger,
	kubeclient client.Client, vSwitches *commonapi.VSwitches,
	timeout time.Duration,
) (err error) {
	var errList []error

	for _, vsw := range *vSwitches {
		// 指定已有的 vswitch 不需要删除
		if vsw.ID != "" {
			continue
		}

		vswitch := &csv1alpha1.Vswitch{}
		vswitchKey := types.NamespacedName{
			Name: vsw.Name,
		}
		_log := log.WithValues("Vswitch", vswitch.Name)

		if err = kubeclient.Get(ctx, vswitchKey, vswitch); err != nil {
			if !apierrors.IsNotFound(err) {
				_log.Error(err, "failed to get vswitch(to delete)")
				errList = append(errList, err)
			}
			_log.Info("vswitch doesn't exist now")
		} else {
			// 判断是否正在删除中
			if !vswitch.ObjectMeta.DeletionTimestamp.IsZero() {
				_log.Info("Vswitch deleting")
				// 判断超时
				metav1Time := metav1.NewTime(time.Now().Add(-timeout))
				if vswitch.ObjectMeta.DeletionTimestamp.Before(&metav1Time) {
					errList = append(errList, ErrorTimeout)
				}
				continue
			}

			if err = kubeclient.Delete(ctx, vswitch); err != nil {
				_log.Error(err, "failed to delete vswitch")
				errList = append(errList, err)
				continue
			}
			_log.Info("Vswitch delete success")
		}
	}

	return utilerrors.NewAggregate(errList)
}
