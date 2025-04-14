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
	"strings"
	"time"

	ess20220222 "github.com/alibabacloud-go/ess-20220222/v2/client"
	teautils "github.com/alibabacloud-go/tea-utils/v2/service"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	upjetresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expclusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"cluster-api-provider-aliyun/api/alibabacloud/v1beta1"
	controlplanev1beta2 "cluster-api-provider-aliyun/api/controlplane/v1beta2"
	csv1alpha1 "cluster-api-provider-aliyun/api/cs/v1alpha1"
	machinepoolv1beta2 "cluster-api-provider-aliyun/api/infrastructure/v1beta2"
	"cluster-api-provider-aliyun/internal/clients"
	"cluster-api-provider-aliyun/internal/common"
)

// AliyunManagedMachinePoolReconciler reconciles a AliyunManagedMachinePool object
type AliyunManagedMachinePoolReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	DeleteTimeout time.Duration
}

// 手动添加 MachinePool, KubernetesNodePool 的权限列表
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools/finalizers,verbs=update

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmanagedmachinepools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmanagedmachinepools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmanagedmachinepools/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AliyunManagedMachinePool object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *AliyunManagedMachinePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(5).Info("Reconcile AliyunManagedMachinePool")

	aliyunPool := &machinepoolv1beta2.AliyunManagedMachinePool{}
	if err := r.Client.Get(ctx, req.NamespacedName, aliyunPool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// MachinePool.spec.template.spec.infrastructureRef 使用名称与 aliyunPool 建立联系,
	// 借鉴自 cluster-api-provider-aws v2.4.0, 符合 cluster-api 规范,
	// 因此这里不再建立 uid 索引器.
	machinePool, err := getOwnerMachinePool(ctx, r.Client, aliyunPool.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to retrieve owner MachinePool from the API Server")
		return ctrl.Result{}, err
	}
	if machinePool == nil {
		log.Info("MachinePool Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("MachinePool", klog.KObj(machinePool))

	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machinePool.ObjectMeta)
	if err != nil {
		log.Info("Failed to retrieve Cluster from MachinePool")
		return reconcile.Result{}, nil
	}

	if annotations.IsPaused(cluster, aliyunPool) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// 函数返回时执行 update 行为
	defer func() {
		old := aliyunPool.DeepCopy()
		if deferr := r.Client.Status().Update(ctx, aliyunPool); deferr != nil {
			err = deferr
			return
		}
		aliyunPool.Spec = old.Spec
		aliyunPool.ObjectMeta.Finalizers = old.ObjectMeta.Finalizers
		if deferr := r.Client.Update(ctx, aliyunPool, &client.UpdateOptions{}); deferr != nil {
			err = deferr
			return
		}
	}()

	if !aliyunPool.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, log, aliyunPool)
	}

	log = log.WithValues("cluster", klog.KObj(cluster))

	// MachinePool 的流程依赖 control plane ready(endpoint已设置, clusterId已生成).
	controlPlaneKey := client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      cluster.Spec.ControlPlaneRef.Name,
	}
	aliyunControlPlane := &controlplanev1beta2.AliyunManagedControlPlane{}
	if err := r.Client.Get(ctx, controlPlaneKey, aliyunControlPlane); err != nil {
		log.Info("Failed to retrieve ControlPlane from MachinePool")
		return reconcile.Result{}, nil
	}

	if !aliyunControlPlane.Status.Ready {
		log.Info("Control plane is not ready yet")
		conditions.MarkFalse(
			aliyunPool,
			machinepoolv1beta2.AliyunManagedControlPlaneReadyCondition,
			machinepoolv1beta2.WaitingForAliyunManagedControlPlaneReason,
			clusterv1.ConditionSeverityInfo, "",
		)
		return ctrl.Result{}, nil
	}
	conditions.MarkTrue(aliyunPool, machinepoolv1beta2.AliyunManagedControlPlaneReadyCondition)

	// sdk client
	if err := r.setupSDKClient(ctx, cluster); err != nil {
		log.Error(err, "setup SDK client failed")
		conditions.MarkFalse(
			aliyunPool,
			machinepoolv1beta2.AliyunManagedMachinePoolReadyCondition,
			machinepoolv1beta2.SetupSDKClientFailedReason,
			clusterv1.ConditionSeverityInfo,
			err.Error(),
		)
		return ctrl.Result{}, err
	}
	if controllerutil.AddFinalizer(aliyunPool, machinepoolv1beta2.ManagedMachinePoolFinalizer) {
		if err := r.Client.Update(ctx, aliyunPool, &client.UpdateOptions{}); err != nil {
			return ctrl.Result{}, err
		}
	}

	// managedKubernetes
	managedKubernetes := &csv1alpha1.ManagedKubernetes{}
	managedKubernetesKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if err := r.Client.Get(ctx, managedKubernetesKey, managedKubernetes); err != nil {
		// if !apierrors.IsNotFound(err) {
		// }
		return ctrl.Result{}, errors.Wrapf(
			err, "failed to get ManagedKubernetes for KubernetesNodePool",
		)
	}

	// vswitch
	vswitchIDs, err := common.ReconcileVSwitch(
		ctx, log, r.Client,
		aliyunPool, aliyunPool.Spec.AckNodePoolName,
		managedKubernetes.Spec.ResourceSpec,
		&aliyunPool.Spec.ScalingGroup.VSwitches,
		*managedKubernetes.Status.AtProvider.VPCID,
	)
	if err != nil {
		conditions.MarkFalse(
			aliyunPool,
			machinepoolv1beta2.VSwitchReconcileReadyCondition,
			machinepoolv1beta2.VSwitchReconcileFailedReason,
			clusterv1.ConditionSeverityError,
			err.Error(),
		)
		return ctrl.Result{}, errors.Wrapf(
			err, "failed to reconcile VSwitch for AliyunManagedControlPlane %s/%s",
			aliyunPool.Namespace, aliyunPool.Name,
		)
	}
	conditions.MarkTrue(aliyunPool, machinepoolv1beta2.VSwitchReconcileReadyCondition)

	kubernetesNodePool := &csv1alpha1.KubernetesNodePool{}
	if err := r.reconcileKubernetesNodePool(
		ctx, log, aliyunControlPlane, managedKubernetes,
		machinePool, aliyunPool, kubernetesNodePool, vswitchIDs,
	); err != nil {
		conditions.MarkFalse(
			aliyunControlPlane,
			machinepoolv1beta2.KubernetesNodePoolReconcileReadyCondition,
			machinepoolv1beta2.KubernetesNodePoolReconcileFailedReason,
			clusterv1.ConditionSeverityError,
			err.Error(),
		)
		return ctrl.Result{}, errors.Wrapf(
			err, "failed to reconcile KubernetesNodePool for AliyunManagedMachinePool %s/%s",
			aliyunPool.Namespace, aliyunPool.Name,
		)
	}
	conditions.MarkTrue(aliyunPool, machinepoolv1beta2.KubernetesNodePoolReconcileReadyCondition)
	r.setStatus(ctx, log, aliyunPool, kubernetesNodePool)

	log.V(5).Info("Successfully reconciled AliyunManagedMachinePool")
	return ctrl.Result{}, nil
}

func (r *AliyunManagedMachinePoolReconciler) reconcileKubernetesNodePool(
	ctx context.Context,
	log logr.Logger,
	aliyunControlPlane *controlplanev1beta2.AliyunManagedControlPlane,
	managedKubernetes *csv1alpha1.ManagedKubernetes,
	machinePool *expclusterv1.MachinePool,
	aliyunPool *machinepoolv1beta2.AliyunManagedMachinePool,
	kubernetesNodePool *csv1alpha1.KubernetesNodePool,
	vswitchIDs []*string,
) error {
	// todo: 创建 NodePool的正常流程
	kubernetesNodePoolKey := types.NamespacedName{
		Namespace: aliyunPool.Namespace,
		Name:      aliyunPool.Name,
	}
	var exist bool
	if err := r.Client.Get(ctx, kubernetesNodePoolKey, kubernetesNodePool); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		exist = true
	}

	// 节点池的 region 无法自主选择
	aliyunPool.Spec.Region = aliyunControlPlane.Spec.Region
	aliyunPool.Spec.ClusterID = aliyunControlPlane.Status.ClusterID
	replicas := float64(*machinePool.Spec.Replicas)
	aliyunPool.Spec.ScalingGroup.DesiredSize = &replicas

	if !exist {
		kubernetesNodePool = &csv1alpha1.KubernetesNodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      aliyunPool.Name,
				Namespace: aliyunPool.Namespace,
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
			Spec: csv1alpha1.KubernetesNodePoolSpec{
				ResourceSpec: managedKubernetes.Spec.ResourceSpec,
				ForProvider: csv1alpha1.KubernetesNodePoolParameters{
					ClusterID:       &aliyunPool.Spec.ClusterID,
					Name:            &aliyunPool.Spec.AckNodePoolName,
					ResourceGroupID: &aliyunPool.Spec.ResourceGroupID,
					VswitchIds:      vswitchIDs,
					InstanceTypes:   aliyunPool.Spec.ScalingGroup.InstanceTypes,
					// todo: KMSEncryptedPassword 与 KeyName 冲突
					// KMSEncryptedPassword:       &aliyunPool.Spec.ScalingGroup.Password,

					// 在 terraform-provider 版本由 1.218.0 变更为 1.223.2 后,
					// KubernetesNodePool 在同步时就会出现 DesiredSize 与 ScalingConfig 冲突的情况,
					// 两者不能同时出现, 于是只能选择弃用 DesiredSize.
					DesiredSize: nil,
					ScalingConfig: []csv1alpha1.ScalingConfigParameters{
						{
							MaxSize: aliyunPool.Spec.ScalingGroup.DesiredSize,
							MinSize: aliyunPool.Spec.ScalingGroup.DesiredSize,
						},
					},
					SystemDiskCategory:         &aliyunPool.Spec.ScalingGroup.SystemDiskCategory,
					SystemDiskSize:             aliyunPool.Spec.ScalingGroup.SystemDiskSize,
					SystemDiskPerformanceLevel: &aliyunPool.Spec.ScalingGroup.SystemDiskPerformanceLevel,
					// ImageType:                  &aliyunPool.Spec.ScalingGroup.ImageType,
					SecurityGroupIds: aliyunPool.Spec.ScalingGroup.SecurityGroupIDs,
					RuntimeName:      &aliyunPool.Spec.ScalingGroup.KubernetesConfig.RuntimeName,
					RuntimeVersion:   &aliyunPool.Spec.ScalingGroup.KubernetesConfig.RuntimeVersion,
					UserData:         &aliyunPool.Spec.ScalingGroup.KubernetesConfig.UserData,
					DataDisks:        aliyunPool.Spec.ScalingGroup.DataDisks,
					Tags:             aliyunPool.Spec.ScalingGroup.KubernetesConfig.Tags,
					Labels:           aliyunPool.Spec.ScalingGroup.KubernetesConfig.Labels,
					Taints:           aliyunPool.Spec.ScalingGroup.KubernetesConfig.Taints,
				},
			},
		}
		// ImageType 为空时, kubernetesNodePool.Spec.ForProvider.ImageType 为 nil, 不能赋值为空
		if aliyunPool.Spec.ScalingGroup.ImageType != "" {
			kubernetesNodePool.Spec.ForProvider.ImageType = &aliyunPool.Spec.ScalingGroup.ImageType
		}
		if err := r.ensurePasswordAndKeyName(ctx, aliyunPool, kubernetesNodePool); err != nil {
			return fmt.Errorf("ensurePasswordAndKeyName failed: %s", err)
		}
		if err := r.Client.Create(ctx, kubernetesNodePool, &client.CreateOptions{}); err != nil {
			return fmt.Errorf("KubernetesNodePool create failed: %s", err)
		}
		conditions.MarkTrue(aliyunPool, machinepoolv1beta2.AliyunManagedMachinePoolCreatingCondition)
	} else {
		kubernetesNodePool.Spec.ForProvider.Name = &aliyunPool.Spec.AckNodePoolName
		kubernetesNodePool.Spec.ForProvider.VswitchIds = vswitchIDs
		kubernetesNodePool.Spec.ForProvider.ResourceGroupID = &aliyunPool.Spec.ResourceGroupID
		kubernetesNodePool.Spec.ForProvider.KeyName = &aliyunPool.Spec.ScalingGroup.KeyName
		// todo: KMSEncryptedPassword 与 KeyName 冲突
		// kubernetesNodePool.Spec.ForProvider.KMSEncryptedPassword = &aliyunPool.Spec.ScalingGroup.Password
		kubernetesNodePool.Spec.ForProvider.InstanceTypes = aliyunPool.Spec.ScalingGroup.InstanceTypes
		kubernetesNodePool.Spec.ForProvider.DesiredSize = nil
		kubernetesNodePool.Spec.ForProvider.ScalingConfig = []csv1alpha1.ScalingConfigParameters{
			{
				MaxSize: aliyunPool.Spec.ScalingGroup.DesiredSize,
				MinSize: aliyunPool.Spec.ScalingGroup.DesiredSize,
			},
		}
		kubernetesNodePool.Spec.ForProvider.SystemDiskCategory = &aliyunPool.Spec.ScalingGroup.SystemDiskCategory
		kubernetesNodePool.Spec.ForProvider.SystemDiskSize = aliyunPool.Spec.ScalingGroup.SystemDiskSize
		kubernetesNodePool.Spec.ForProvider.SystemDiskPerformanceLevel = &aliyunPool.Spec.ScalingGroup.SystemDiskPerformanceLevel
		kubernetesNodePool.Spec.ForProvider.DataDisks = aliyunPool.Spec.ScalingGroup.DataDisks

		kubernetesNodePool.Spec.ForProvider.RuntimeName = &aliyunPool.Spec.ScalingGroup.KubernetesConfig.RuntimeName
		kubernetesNodePool.Spec.ForProvider.RuntimeVersion = &aliyunPool.Spec.ScalingGroup.KubernetesConfig.RuntimeVersion
		kubernetesNodePool.Spec.ForProvider.UserData = &aliyunPool.Spec.ScalingGroup.KubernetesConfig.UserData
		// 如下3个字段都是数组类型, 本来以为在更新时, 会出现相同key值覆盖, 不同key值合并, 无法删除的情况,
		// 但实际则是可以删除, 猜测是ACK平台预设了一些值, 而开放给使用者的部分会被全部替换.
		// 注意, 这3者的变更不会影响已有节点/ecs, 只有新增节点才会使用变更后的值.
		kubernetesNodePool.Spec.ForProvider.Tags = aliyunPool.Spec.ScalingGroup.KubernetesConfig.Tags     // 节点池中每个ecs的标签(与ACK不在一处)
		kubernetesNodePool.Spec.ForProvider.Labels = aliyunPool.Spec.ScalingGroup.KubernetesConfig.Labels // 节点池中所有节点的label标签
		kubernetesNodePool.Spec.ForProvider.Taints = aliyunPool.Spec.ScalingGroup.KubernetesConfig.Taints // 节点池中所有节点的taint污点

		if err := r.ensurePasswordAndKeyName(ctx, aliyunPool, kubernetesNodePool); err != nil {
			return fmt.Errorf("ensurePasswordAndKeyName failed: %s", err)
		}

		if err := r.Client.Update(ctx, kubernetesNodePool, &client.UpdateOptions{}); err != nil {
			return fmt.Errorf("KubernetesNodePool update failed: %s", err)
		}
		// todo: no need to Update
		if !conditions.IsTrue(aliyunPool, machinepoolv1beta2.AliyunManagedMachinePoolCreatingCondition) {
			conditions.MarkTrue(aliyunPool, machinepoolv1beta2.AliyunManagedMachinePoolUpdatingCondition)
		}
		log.Info("KubernetesNodePool update success")
	}

	return nil
}

func (r *AliyunManagedMachinePoolReconciler) setupSDKClient(
	ctx context.Context, cluster *clusterv1.Cluster,
) (err error) {
	if clients.AliyunCreds.AccessKey != "" && clients.AliyunCreds.SecretKey != "" {
		return
	}
	managedKubernetes := &csv1alpha1.ManagedKubernetes{}
	managedKubernetesKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if err := r.Client.Get(ctx, managedKubernetesKey, managedKubernetes); err != nil {
		// if !apierrors.IsNotFound(err) {
		// }
		return fmt.Errorf("get ManagedKubernetes failed: %s", err)
	}
	pc := &v1beta1.ProviderConfig{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: managedKubernetes.Spec.ProviderConfigReference.Name}, pc); err != nil {
		return fmt.Errorf("get ProviderConfig failed: %s", err)
	}

	data, err := resource.CommonCredentialExtractor(ctx, pc.Spec.Credentials.Source, r.Client, pc.Spec.Credentials.CommonCredentialSelectors)
	if err != nil {
		return fmt.Errorf("extract Credentials failed: %s", err)
	}
	if err := json.Unmarshal(data, &clients.AliyunCreds); err != nil {
		return fmt.Errorf("unmarshal Credentials failed: %s", err)
	}
	return
}

func (r *AliyunManagedMachinePoolReconciler) setStatus(
	_ context.Context,
	log logr.Logger,
	aliyunPool *machinepoolv1beta2.AliyunManagedMachinePool,
	kubernetesNodePool *csv1alpha1.KubernetesNodePool,
) (err error) {
	{
		// 根据 crossplane 资源状态设置 condition, 但对 control plane 的就绪状态并没有决定性影响
		var syncedCondition = kubernetesNodePool.GetCondition(xpv1.TypeSynced)
		if syncedCondition.Message != "" {
			conditions.MarkFalse(
				aliyunPool,
				csv1alpha1.CrossPlaneReadyCondition,
				csv1alpha1.CrossPlaneFailedCondition,
				clusterv1.ConditionSeverityError,
				syncedCondition.Message,
			)
			aliyunPool.Status.FailureMessage = syncedCondition.Message
		} else {
			conditions.MarkTrue(aliyunPool, csv1alpha1.CrossPlaneReadyCondition)
		}
		var asyncCondition = kubernetesNodePool.GetCondition(upjetresource.TypeAsyncOperation)
		if asyncCondition.Message != "" {
			conditions.MarkFalse(
				aliyunPool,
				csv1alpha1.UpjetProviderReadyCondition,
				csv1alpha1.UpjetProviderFailedCondition,
				clusterv1.ConditionSeverityError,
				asyncCondition.Message,
			)
			aliyunPool.Status.FailureMessage = asyncCondition.Message
		} else {
			conditions.MarkTrue(aliyunPool, csv1alpha1.UpjetProviderReadyCondition)
		}
		var lastAsyncCondition = kubernetesNodePool.GetCondition(upjetresource.TypeLastAsyncOperation)
		if lastAsyncCondition.Message != "" {
			conditions.MarkFalse(
				aliyunPool,
				csv1alpha1.UpjetProviderReadyCondition,
				csv1alpha1.UpjetProviderFailedCondition,
				clusterv1.ConditionSeverityError,
				lastAsyncCondition.Message,
			)
			// aliyunPool.Status.FailureMessage = lastAsyncCondition.Message
		} else {
			conditions.MarkTrue(aliyunPool, csv1alpha1.UpjetProviderReadyCondition)
		}
	}

	var readyCondition = kubernetesNodePool.GetCondition(xpv1.TypeReady)
	if readyCondition.Status != corev1.ConditionTrue {
		aliyunPool.Status.Ready = false
		// todo: 设置失败原因
		return
	}

	log.Info("KubernetesNodePool is ready")

	id := strings.Split(*kubernetesNodePool.Status.AtProvider.ID, ":")
	_, nodePoolID := id[0], id[1] // 分别是集群id, 节点池id
	aliyunPool.Status.Ready = true
	aliyunPool.Status.AckNodePoolID = nodePoolID
	if kubernetesNodePool.Spec.ForProvider.DesiredSize != nil {
		aliyunPool.Status.Replicas = int32(*kubernetesNodePool.Status.AtProvider.DesiredSize)
	} else if len(kubernetesNodePool.Status.AtProvider.ScalingConfig) != 0 {
		scalingConfig := kubernetesNodePool.Status.AtProvider.ScalingConfig[0]
		aliyunPool.Status.Replicas = int32(*scalingConfig.MinSize)
	}
	// 按照设计, aliyunPool status 块没有 SecurityGroupIDs 的展示列表字段, 所以先回写到 spec 块.
	if len(aliyunPool.Spec.ScalingGroup.SecurityGroupIDs) == 0 {
		aliyunPool.Spec.ScalingGroup.SecurityGroupIDs = kubernetesNodePool.Status.AtProvider.SecurityGroupIds
	}

	describeScalingInstancesRequest := &ess20220222.DescribeScalingInstancesRequest{
		RegionId:       &aliyunPool.Spec.Region,
		ScalingGroupId: kubernetesNodePool.Status.AtProvider.ScalingGroupID,
	}
	// ACK 集群中, 每个node节点都有 .spec.provider 字段, 需要收集起来, 供 cluster-api 比对.
	var providerIDList []string
	aliyunSDKClient, err := clients.CreateSDKClient(aliyunPool.Spec.Region)
	if err != nil {
		return
	}
	scalingInstancesResponse, err := aliyunSDKClient.DescribeScalingInstancesWithOptions(
		describeScalingInstancesRequest, &teautils.RuntimeOptions{},
	)
	if err != nil {
		return err
	}
	for _, instance := range scalingInstancesResponse.Body.ScalingInstances {
		if instance != nil && instance.InstanceId != nil && *instance.InstanceId != "" {
			providerIDList = append(providerIDList, fmt.Sprintf(
				"%s.%s", aliyunPool.Spec.Region, *instance.InstanceId,
			))
		} else {
			log.Error(
				fmt.Errorf("invalid ecs instance"), "", "instance", instance,
			)
		}
	}
	aliyunPool.Spec.ProviderIDList = providerIDList

	conditions.MarkTrue(aliyunPool, machinepoolv1beta2.AliyunManagedMachinePoolReadyCondition)
	return
}

func (r *AliyunManagedMachinePoolReconciler) reconcileDelete(
	ctx context.Context,
	log logr.Logger,
	aliyunPool *machinepoolv1beta2.AliyunManagedMachinePool,
) (_ ctrl.Result, err error) {
	log = log.WithValues("operation", "delete")

	kubernetesNodePool := &csv1alpha1.KubernetesNodePool{}
	kubernetesNodePoolKey := types.NamespacedName{
		Namespace: aliyunPool.Namespace,
		Name:      aliyunPool.Name,
	}
	if err := r.Client.Get(ctx, kubernetesNodePoolKey, kubernetesNodePool); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	} else {
		log = log.WithValues("KubernetesNodePool", kubernetesNodePool.Name)
		// 判断是否正在删除中
		if !kubernetesNodePool.ObjectMeta.DeletionTimestamp.IsZero() {
			log.V(3).Info("KubernetesNodePool deleting")
			return ctrl.Result{RequeueAfter: time.Second * 20}, nil
		}

		if err := r.Client.Delete(ctx, kubernetesNodePool); err != nil {
			return ctrl.Result{RequeueAfter: time.Second * 20}, err
		}
		log.Info("KubernetesNodePool delete success")
		return ctrl.Result{}, nil
	}

	// 待清理完关联资源后, 移除 finalizer
	log.Info("KubernetesNodePool doesn't exist now")

	err = common.ReconcileRemoveVSwitch(
		ctx, log, r.Client, &aliyunPool.Spec.ScalingGroup.VSwitches, r.DeleteTimeout,
	)
	if err != nil {
		for _, e := range err.(utilerrors.Aggregate).Errors() {
			if e == common.ErrorTimeout {
				continue
			}
			return ctrl.Result{}, err
		}
	}

	log.Info("VSwitch of KubernetesNodePool doesn't exist now")

	controllerutil.RemoveFinalizer(aliyunPool, machinepoolv1beta2.ManagedMachinePoolFinalizer)
	log.Info("AliyunManagedMachinePool remove finalizer success")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AliyunManagedMachinePoolReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) (err error) {
	// 添加 Indexer 索引器, 通过 uid 信息找到 aliyunPool 资源.
	if err = mgr.GetFieldIndexer().IndexField(ctx,
		&machinepoolv1beta2.AliyunManagedMachinePool{},
		common.IndexAliyunPoolByUID,
		r.indexAliyunPoolByUID,
	); err != nil {
		return err
	}

	if err = mgr.GetFieldIndexer().IndexField(ctx,
		&csv1alpha1.KubernetesNodePool{},
		common.IndexNodePoolByAliyunPoolUID,
		r.indexNodePoolByAliyunPoolUID,
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&machinepoolv1beta2.AliyunManagedMachinePool{}).
		// WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		Watches(
			// 监听 machine pool 副本数变更, 加快响应
			&expclusterv1.MachinePool{},
			handler.EnqueueRequestsFromMapFunc(
				r.mapMachinePoolToAliyunPool(),
			),
		).
		Watches(
			// 监听 control plane 变更为 ready, 缩短创建时间.
			&controlplanev1beta2.AliyunManagedControlPlane{},
			handler.EnqueueRequestsFromMapFunc(
				r.mapAliyunControlPlaneToAliyunPool(),
			),
		).
		// 由于 KubernetesNodePool 本身是 cluster scope 的, 其 ownerRef 中也就不包含 namespace 信息了,
		// 因此ta的变动将无法被正确地映射成 Request.NamespacedName 信息(namespace值将为""), 无法查询到.
		// ManagedKubernetes 同理.
		// Owns(&csv1alpha1.KubernetesNodePool{}).
		Watches(
			// 监听 KubernetesNodePool 就绪/删除, 缩短创建时间.
			&csv1alpha1.KubernetesNodePool{},
			handler.EnqueueRequestsFromMapFunc(
				r.mapNodePoolToAliyunPool(),
			),
		).
		Complete(r)
}
