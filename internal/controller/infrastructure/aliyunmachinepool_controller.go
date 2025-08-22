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
	"encoding/json"
	"fmt"
	alibabacloudv1beta1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/alibabacloud/v1beta1"
	essv1alpha1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/ess/v1alpha1"
	infrastructurev1beta2 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/infrastructure/v1beta2"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/clients"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/controller/controlplane"
	ess20220222 "github.com/alibabacloud-go/ess-20220222/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterexpv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	capiexputil "sigs.k8s.io/cluster-api/exp/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
	"time"
)

// AliyunMachinePoolReconciler reconciles a AliyunMachinePool object
type AliyunMachinePoolReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CredentialSecret *controlplane.CredentialSecret
}

const (
	RegionAnnotation = "alibabacloud.alibabacloud.com/region"
)

// 读写 Secret
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmachinepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmachinepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmachinepools/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AliyunMachinePool object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *AliyunMachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	log := ctrl.LoggerFrom(ctx)
	log.V(5).Info("Reconcile AliyunMachinePool")

	// Get the AliyunMachinePool
	mp := &infrastructurev1beta2.AliyunMachinePool{}
	if err := r.Client.Get(ctx, req.NamespacedName, mp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log = log.WithValues("AliyunMachinePool", mp.Name)

	//if _, err := r.syncUserData(ctx, mp); err != nil {
	//	return ctrl.Result{}, err
	//}

	if !mp.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, log, mp)
	}

	if controllerutil.AddFinalizer(mp, infrastructurev1beta2.AliyunMachinePoolFinalizer) {
		if err := r.Client.Update(ctx, mp); err != nil {
			return ctrl.Result{}, err
		}
	}

	pc, err := r.reconcileProviderConfig(ctx, log, mp)
	if err != nil {
		// 这里也可以给 mp 设置一个 Condition 表示凭证错误
		return ctrl.Result{}, err
	}

	// 用 ProviderConfig 去对齐 ESS ScalingGroup（Upjet 资源）
	if res, err := r.reconcileScalingGroup(ctx, mp, pc.Name); err != nil {
		return res, err
	}

	// 读取 SG，看是否已就绪并拿到 ID，再去确保/更新 ScalingConfiguration
	sgName := fmt.Sprintf("%s-%s", mp.Name, strings.ToLower(string(mp.UID))[:8])
	curSG := &essv1alpha1.ScalingGroup{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: sgName}, curSG); err != nil {
		return ctrl.Result{}, err
	}
	if curSG.Status.AtProvider.ID == nil {
		// SG 还没拿到 ID，我们会 watch ScalingGroup 反向触发 machine pool 来触发 SGC（ScalingConfiguration） 的创建
		return ctrl.Result{}, nil
	}
	if _, err := r.reconcileScalingConfiguration(ctx, mp, pc.Name, *curSG.Status.AtProvider.ID); err != nil {
		return ctrl.Result{}, err
	}

	scName := fmt.Sprintf("%s-sc-%s", mp.Name, strings.ToLower(string(mp.UID))[:8])
	sc := &essv1alpha1.ScalingConfiguration{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: scName}, sc); err != nil {
		return ctrl.Result{}, err
	}
	if sc.Status.AtProvider.ID == nil {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	region := mp.Annotations[RegionAnnotation]
	if region == "" {
		region = r.CredentialSecret.Region
	}
	sdkClient, err := clients.CreateSDKClient(region)
	if err != nil {
		return ctrl.Result{}, err
	}
	describeScalingInstancesRequest := &ess20220222.DescribeScalingInstancesRequest{ScalingGroupId: tea.String(*curSG.Status.AtProvider.ID)}
	describeScalingInstancesResp, err := sdkClient.DescribeScalingInstances(describeScalingInstancesRequest)
	if err != nil {
		return ctrl.Result{}, err
	}
	instances := describeScalingInstancesResp.Body.ScalingInstances
	replicas, readyReplicas, err := calcReplicas(instances)
	if err != nil {
		return ctrl.Result{}, err
	}

	mp.Status.ScalingGroupID = *curSG.Status.AtProvider.ID
	mp.Status.ScalingConfigurationID = tea.StringValue(sc.Status.AtProvider.ID)
	mp.Status.Replicas = replicas
	mp.Status.ReadyReplicas = readyReplicas

	conditions.MarkTrue(mp, infrastructurev1beta2.ScalingConfigurationReadyCondition)
	if replicas == readyReplicas {
		conditions.MarkTrue(mp, infrastructurev1beta2.ScalingConfigurationInstanceReadyCondition)
		if err := r.Client.Status().Update(ctx, mp); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		conditions.MarkFalse(
			mp,
			infrastructurev1beta2.ScalingConfigurationInstanceReadyCondition,
			"",
			clusterv1.ConditionSeverityInfo,
			"waiting for nodes",
		)
		if err := r.Client.Status().Update(ctx, mp); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return ctrl.Result{}, nil
}

// 将 AliyunMachinePool.Spec.ScalingGroup 映射成 aliyun 的 ScalingGroup（Upjet）
func (r *AliyunMachinePoolReconciler) reconcileScalingGroup(
	ctx context.Context,
	mp *infrastructurev1beta2.AliyunMachinePool,
	providerConfigName string,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	labels := map[string]string{
		"infra.alibabacloud.com-owned-by-mp-ns":   mp.Namespace,
		"infra.alibabacloud.com-owned-by-mp-name": mp.Name,
	}

	// 生成一个全局唯一的名字：<pool-name>-<uid前8位>
	sgName := fmt.Sprintf("%s-%s", mp.Name, strings.ToLower(string(mp.UID))[:8])

	want := essv1alpha1.ScalingGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:   sgName,
			Labels: labels,
		},
		Spec: essv1alpha1.ScalingGroupSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: providerConfigName},
			},
			ForProvider: essv1alpha1.ScalingGroupParameters{
				ScalingGroupName: &mp.Spec.ScalingGroup.ScalingGroupName,
				MinSize:          f64Ptr(mp.Spec.ScalingGroup.MinSize),
				MaxSize:          f64Ptr(mp.Spec.ScalingGroup.MaxSize),
				DesiredCapacity:  f64Ptr(mp.Spec.ScalingGroup.DesiredCapacity),
				VswitchIds:       toPtrSlice(mp.Spec.ScalingGroup.VSwitchIDs),
				LoadbalancerIds:  toPtrSlice(mp.Spec.ScalingGroup.LoadBalancerIDs),
			},
		},
	}

	cur := &essv1alpha1.ScalingGroup{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: sgName}, cur); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err := r.Client.Create(ctx, &want); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Created ESS ScalingGroup", "name", sgName)
		return ctrl.Result{}, nil
	}

	// 已存在 → 合并 label，比较关键字段并更新
	changed := false

	if cur.Labels == nil {
		cur.Labels = map[string]string{}
	}
	for k, v := range labels {
		if cur.Labels[k] != v {
			cur.Labels[k] = v
			changed = true
		}
	}

	if !providerRefEqual(cur.Spec.ResourceSpec.ProviderConfigReference, want.Spec.ResourceSpec.ProviderConfigReference) {
		cur.Spec.ResourceSpec.ProviderConfigReference = want.Spec.ResourceSpec.ProviderConfigReference
		changed = true
	}

	if !equalScalingGroupParams(cur.Spec.ForProvider, want.Spec.ForProvider) {
		cur.Spec.ForProvider = want.Spec.ForProvider
		changed = true
	}

	if changed {
		if err := r.Client.Update(ctx, cur); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Updated ESS ScalingGroup", "name", cur.Name)
	}

	return ctrl.Result{}, nil
}

func (r *AliyunMachinePoolReconciler) reconcileProviderConfig(
	ctx context.Context,
	log logr.Logger,
	mp *infrastructurev1beta2.AliyunMachinePool,
) (*alibabacloudv1beta1.ProviderConfig, error) {

	// 决定 region：优先注解；也可改成从 mp.Spec.Region / Owner Cluster 推导
	region := mp.Annotations[RegionAnnotation]
	if region == "" && r.CredentialSecret.Region != "" {
		region = r.CredentialSecret.Region
	}
	if region == "" {
		return nil, fmt.Errorf("region not set: add annotation %q or configure Reconciler.CredentialSecret.Region", RegionAnnotation)
	}

	// 在注册 upjet provider 时配置 accessKey 和 SecretKey
	clients.AliyunCreds.AccessKey = r.CredentialSecret.AccessKey
	clients.AliyunCreds.SecretKey = r.CredentialSecret.SecretKey
	clients.AliyunCreds.Region = region

	// Secret 名约定为: aliyun-<region>，放在 r.CredentialSecret.Namespace
	secretKey := types.NamespacedName{
		Namespace: r.CredentialSecret.Namespace,
		Name:      fmt.Sprintf("aliyun-%s", region),
	}
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, secretKey, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		// 创建 secret（字段名 r.CredentialSecret.Key，值为 JSON：{access_key, secret_key, region}）
		log.Info("creating Secret for provider credentials", "secret", secretKey.String())
		credMap := map[string]string{
			clients.KeyAccessKey: r.CredentialSecret.AccessKey,
			clients.KeySecretKey: r.CredentialSecret.SecretKey,
			clients.KeyRegion:    region,
		}
		payload, err := json.Marshal(credMap)
		if err != nil {
			return nil, err
		}
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretKey.Name,
				Namespace: secretKey.Namespace,
			},
			Data: map[string][]byte{
				r.CredentialSecret.Key: payload,
			},
			Type: corev1.SecretTypeOpaque,
		}
		if err := r.Client.Create(ctx, secret); err != nil {
			return nil, err
		}
	}

	// ProviderConfig 用 region 作为名字
	pc := &alibabacloudv1beta1.ProviderConfig{}
	pcKey := types.NamespacedName{Name: region}
	if err := r.Client.Get(ctx, pcKey, pc); err == nil {
		return pc, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}

	log.Info("creating ProviderConfig", "name", region)
	pc = &alibabacloudv1beta1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: region,
		},
		Spec: alibabacloudv1beta1.ProviderConfigSpec{
			Credentials: alibabacloudv1beta1.ProviderCredentials{
				Source: xpv1.CredentialsSourceSecret,
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{
						SecretReference: xpv1.SecretReference{
							Namespace: secretKey.Namespace,
							Name:      secretKey.Name,
						},
						Key: r.CredentialSecret.Key,
					},
				},
			},
		},
	}
	if err := r.Client.Create(ctx, pc); err != nil {
		return nil, err
	}
	return pc, nil
}

// 通过我们打在 SG 上的 label  找到 “所属的” AliyunMachinePool
func (r *AliyunMachinePoolReconciler) mapScalingGroupToMachinePool(ctx context.Context, obj client.Object) []ctrl.Request {
	sg, ok := obj.(*essv1alpha1.ScalingGroup)
	if !ok {
		return nil
	}
	ns := sg.GetLabels()["infra.alibabacloud.com-owned-by-mp-ns"]
	name := sg.GetLabels()["infra.alibabacloud.com-owned-by-mp-name"]
	if ns == "" || name == "" {
		return nil
	}
	return []ctrl.Request{{NamespacedName: types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}}}
}

func (r *AliyunMachinePoolReconciler) reconcileScalingConfiguration(
	ctx context.Context,
	mp *infrastructurev1beta2.AliyunMachinePool,
	providerConfigName string,
	scalingGroupID string,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// 基本校验（仍然保留）
	if mp.Spec.ScalingConfiguration.ImageID == "" ||
		len(mp.Spec.ScalingConfiguration.InstanceTypes) == 0 ||
		len(mp.Spec.ScalingConfiguration.SecurityGroupIDs) == 0 {
		return ctrl.Result{}, fmt.Errorf("scalingConfiguration is incomplete: imageId/instanceTypes/securityGroupIds are required")
	}
	labels := map[string]string{
		"infra.alibabacloud.com-owned-by-mp-ns":   mp.Namespace,
		"infra.alibabacloud.com-owned-by-mp-name": mp.Name,
	}
	scName := fmt.Sprintf("%s-sc-%s", mp.Name, strings.ToLower(string(mp.UID))[:8])

	want := essv1alpha1.ScalingConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:   scName,
			Labels: labels,
		},
		Spec: essv1alpha1.ScalingConfigurationSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: providerConfigName},
			},
			ForProvider: essv1alpha1.ScalingConfigurationParameters{
				ScalingGroupID:          &scalingGroupID,
				ImageID:                 ptrIfNotEmpty(mp.Spec.ScalingConfiguration.ImageID),
				InstanceTypes:           toPtrSlice(mp.Spec.ScalingConfiguration.InstanceTypes),
				SecurityGroupIds:        toPtrSlice(mp.Spec.ScalingConfiguration.SecurityGroupIDs),
				UserData:                ptrIfNotEmpty(mp.Spec.ScalingConfiguration.UserData),
				InternetChargeType:      mp.Spec.ScalingConfiguration.InternetChargeType,
				InternetMaxBandwidthIn:  mp.Spec.ScalingConfiguration.InternetMaxBandwidthIn,
				InternetMaxBandwidthOut: mp.Spec.ScalingConfiguration.InternetMaxBandwidthOut,
				SystemDiskCategory:      mp.Spec.ScalingConfiguration.SystemDiskCategory,
				SystemDiskSize:          mp.Spec.ScalingConfiguration.SystemDiskSize,
			},
		},
	}

	cur := &essv1alpha1.ScalingConfiguration{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: scName}, cur); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err := r.Client.Create(ctx, &want); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Created ESS ScalingConfiguration", "name", scName, "sg", scalingGroupID)
		return ctrl.Result{}, nil
	}

	changed := false
	if cur.Labels == nil {
		cur.Labels = map[string]string{}
	}
	for k, v := range labels {
		if cur.Labels[k] != v {
			cur.Labels[k] = v
			changed = true
		}
	}
	if !providerRefEqual(cur.Spec.ResourceSpec.ProviderConfigReference, want.Spec.ResourceSpec.ProviderConfigReference) {
		cur.Spec.ResourceSpec.ProviderConfigReference = want.Spec.ResourceSpec.ProviderConfigReference
		changed = true
	}
	if !equalScalingConfigurationParamsSliceAware(cur.Spec.ForProvider, want.Spec.ForProvider) {
		cur.Spec.ForProvider = want.Spec.ForProvider
		changed = true
	}

	if changed {
		if err := r.Client.Update(ctx, cur); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Updated ESS ScalingConfiguration", "name", cur.Name)
	}
	return ctrl.Result{}, nil
}

func (r *AliyunMachinePoolReconciler) reconcileDelete(
	ctx context.Context,
	log logr.Logger,
	mp *infrastructurev1beta2.AliyunMachinePool,
) (ctrl.Result, error) {
	sgName := fmt.Sprintf("%s-%s", mp.Name, strings.ToLower(string(mp.UID))[:8])
	scName := fmt.Sprintf("%s-sc-%s", mp.Name, strings.ToLower(string(mp.UID))[:8])

	sc := &essv1alpha1.ScalingConfiguration{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: scName}, sc); err == nil {
		// 删除 sg config
		if err := r.Client.Delete(ctx, sc); err != nil {
			return ctrl.Result{}, err
		}
	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	sg := &essv1alpha1.ScalingGroup{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: sgName}, sg); err == nil {
		// 删除 sg
		if err := r.Client.Delete(ctx, sg); err != nil {
			return ctrl.Result{}, err
		}
	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(mp, infrastructurev1beta2.AliyunMachinePoolFinalizer)
	if err := r.Client.Update(ctx, mp); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("AliyunMachinePool delete complete")
	return ctrl.Result{}, nil
}

// getMachinePool returns the CAPI MachinePool owning the AliyunMachinePool using
// owner references or, as a fallback, labels.
func (r *AliyunMachinePoolReconciler) getMachinePool(ctx context.Context, mp *infrastructurev1beta2.AliyunMachinePool) (*clusterexpv1.MachinePool, error) {
	for _, ref := range mp.OwnerReferences {
		if ref.Kind == "MachinePool" && ref.APIVersion == clusterexpv1.GroupVersion.String() {
			m := &clusterexpv1.MachinePool{}
			if err := r.Client.Get(ctx, types.NamespacedName{Namespace: mp.Namespace, Name: ref.Name}, m); err != nil {
				return nil, err
			}
			return m, nil
		}
	}
	return capiexputil.GetMachinePoolByLabels(ctx, r.Client, mp.Namespace, mp.Labels)
}

// syncUserData reads bootstrap data from the MachinePool secret and writes it
// into the AliyunMachinePool's ScalingConfiguration. It returns true if
// UserData was changed.
func (r *AliyunMachinePoolReconciler) syncUserData(ctx context.Context, mp *infrastructurev1beta2.AliyunMachinePool) (bool, error) {
	machinePool, err := r.getMachinePool(ctx, mp)
	if err != nil {
		return false, err
	}
	if machinePool == nil || machinePool.Spec.Template.Spec.Bootstrap.DataSecretName == nil {
		return false, fmt.Errorf("bootstrap data secret name not found")
	}
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: mp.Namespace, Name: *machinePool.Spec.Template.Spec.Bootstrap.DataSecretName}, secret); err != nil {
		return false, err
	}
	userData := string(secret.Data["value"])
	if mp.Spec.ScalingConfiguration.UserData != userData {
		mp.Spec.ScalingConfiguration.UserData = userData
		if err := r.Client.Update(ctx, mp); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AliyunMachinePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1beta2.AliyunMachinePool{}).
		// 关键：监听 ClusterScope 的 ScalingGroup 变化，反向触发对应的 AliyunMachinePool
		Watches(&essv1alpha1.ScalingGroup{}, handler.EnqueueRequestsFromMapFunc(r.mapScalingGroupToMachinePool)).
		Named("infrastructure-aliyunmachinepool").
		Complete(r)
}
