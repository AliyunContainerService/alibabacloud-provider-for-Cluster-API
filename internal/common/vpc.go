package common

import (
	"context"
	"fmt"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	upjetresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonapi "cluster-api-provider-aliyun/api/common"
	csv1alpha1 "cluster-api-provider-aliyun/api/cs/v1alpha1"
)

// reconcileVPC ...
//
// @param object: 一般为 aliyunManagedControlPlane 对象
// @param keyName: .spec.clusterName 信息
func ReconcileVPC(
	ctx context.Context, log logr.Logger,
	kubeclient client.Client, object runtime.Object, keyName string,
	aliyunVPC *commonapi.VPC,
	resourceSpec xpv1.ResourceSpec,
) (vpcID string, err error) {
	// 如果指定已有 vpc 资源.
	if aliyunVPC.ID != "" {
		vpcID = aliyunVPC.ID
		return
	}
	if aliyunVPC.ResourceID != "" && aliyunVPC.UID != "" {
		vpcID = aliyunVPC.ResourceID
		return
	}

	if aliyunVPC.Name == "" {
		// 没有指定 vpc 信息(这是可行的)
		return
	}

	log.WithValues("vpcName", aliyunVPC.Name)

	// 如果未指定, 则根据 VPC 字段自行创建并记录id
	vpcKey := types.NamespacedName{
		Name: aliyunVPC.Name,
	}
	vpc := &csv1alpha1.VPC{}

	var exist bool
	if err = kubeclient.Get(ctx, vpcKey, vpc); err != nil {
		if !apierrors.IsNotFound(err) {
			return
		}
		log.Info("vpc not found, try to create")
	} else {
		// (废弃)如果存在同名资源, 但其 cidr 与配置值不符
		// 如果存在同名资源, 但其属主不是自己, 则报错.
		//
		// 阿里云可以创建同名+同网段+同区域的 VPC/VSwitch 资源, ta们的 id 会不一样,
		// 但是 terraform 不行, 在发现同名资源时会报错, 无法创建.
		ownerRefs := vpc.GetOwnerReferences()
		if len(ownerRefs) == 0 || ownerRefs[0].UID != object.(metav1.Object).GetUID() {
			err = fmt.Errorf(
				"vpc is already exist: %s", aliyunVPC.Name,
			)
			return
		}
		exist = true
	}

	if exist {
		// 假设 vpc 正在创建中.
		if vpc.Status.AtProvider.ID != nil && *vpc.Status.AtProvider.ID != "" {
			vpcID = *vpc.Status.AtProvider.ID
			aliyunVPC.ResourceID = *vpc.Status.AtProvider.ID
			aliyunVPC.UID = string(vpc.UID)
			return
		}
		// 如果出现错误
		// 根据 crossplane 资源状态设置 condition, 但对 control plane 的就绪状态并没有决定性影响
		var syncedCondition = vpc.GetCondition(xpv1.TypeSynced)
		if syncedCondition.Message != "" {
			err = fmt.Errorf("vpc synced failed: %s", syncedCondition.Message)
			return
		}
		var asyncCondition = vpc.GetCondition(upjetresource.TypeAsyncOperation)
		if asyncCondition.Message != "" {
			err = fmt.Errorf("vpc async failed: %s", asyncCondition.Message)
			return
		}
		var lastAsyncCondition = vpc.GetCondition(upjetresource.TypeLastAsyncOperation)
		if lastAsyncCondition.Message != "" {
			err = fmt.Errorf("vpc lastAsync failed: %s", lastAsyncCondition.Message)
			return
		}
		// 如果只是单纯未完成创建
		err = fmt.Errorf("vpc not ready")
	} else {
		gvk := object.GetObjectKind().GroupVersionKind()
		description := commonapi.GenerateVPCDescription(keyName)
		vpc = &csv1alpha1.VPC{
			ObjectMeta: metav1.ObjectMeta{
				Name: aliyunVPC.Name,
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
			Spec: csv1alpha1.VPCSpec{
				ForProvider: csv1alpha1.VPCParameters{
					// Name:        nil,
					VPCName:     &aliyunVPC.Name,
					CidrBlock:   &aliyunVPC.CIDRBlock,
					Description: &description,
				},
				ResourceSpec: resourceSpec,
			},
		}
		if err := kubeclient.Create(ctx, vpc, &client.CreateOptions{}); err != nil {
			return "", fmt.Errorf("vpc create failed: %s", err)
		}
		aliyunVPC.UID = string(vpc.UID)
		log.Info("vpc create success")
		err = fmt.Errorf("vpc not ready")
	}

	return
}

// ReconcileRemoveVPC ...
func ReconcileRemoveVPC(
	ctx context.Context, log logr.Logger,
	kubeclient client.Client, aliyunVPC *commonapi.VPC,
	timeout time.Duration,
) (err error) {
	if aliyunVPC.ResourceID == "" {
		return
	}
	vpc := &csv1alpha1.VPC{}
	vpcKey := types.NamespacedName{
		Name: aliyunVPC.Name,
	}
	log = log.WithValues("VPC", vpc.Name)

	if err = kubeclient.Get(ctx, vpcKey, vpc); err != nil {
		if !apierrors.IsNotFound(err) {
			return
		}
		err = nil
	} else {
		// 判断是否正在删除中
		if !vpc.ObjectMeta.DeletionTimestamp.IsZero() {
			log.Info("VPC deleting")
			// 判断超时
			metav1Time := metav1.NewTime(time.Now().Add(-timeout))
			if vpc.ObjectMeta.DeletionTimestamp.Before(&metav1Time) {
				return ErrorTimeout
			}
			return
		}
		if err = kubeclient.Delete(ctx, vpc); err != nil {
			return
		}
		log.Info("VPC delete success")
	}
	return
}
