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

package controlplane

import (
	"context"
	"fmt"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	upjetresource "github.com/crossplane/upjet/pkg/resource"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	alibabacloudv1beta1 "cluster-api-provider-aliyun/api/alibabacloud/v1beta1"
	controlplanev1beta2 "cluster-api-provider-aliyun/api/controlplane/v1beta2"
	csv1alpha1 "cluster-api-provider-aliyun/api/cs/v1alpha1"
	"cluster-api-provider-aliyun/internal/common"
)

// AliyunManagedControlPlaneReconciler reconciles a AliyunManagedControlPlane object
type AliyunManagedControlPlaneReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CredentialSecret *CredentialSecret
	DeleteTimeout    time.Duration
}

//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=aliyunmanagedcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=aliyunmanagedcontrolplanes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=aliyunmanagedcontrolplanes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AliyunManagedControlPlane object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *AliyunManagedControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(5).Info("Reconcile AliyunManagedMachinePool")

	// Get the control plane instance
	aliyunControlPlane := &controlplanev1beta2.AliyunManagedControlPlane{}
	if err := r.Client.Get(ctx, req.NamespacedName, aliyunControlPlane); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log = log.WithValues("AliyunManagedControlPlane", aliyunControlPlane.Name)

	// Get the cluster
	cluster, err := util.GetOwnerCluster(ctx, r.Client, aliyunControlPlane.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to retrieve owner Cluster from the API Server")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("cluster", cluster.Name)

	// todo: annotations.IsPaused()

	// 函数返回时执行 update 行为
	defer func() {
		old := aliyunControlPlane.DeepCopy()
		if deferr := r.Client.Status().Update(ctx, aliyunControlPlane); deferr != nil {
			err = deferr
			return
		}
		aliyunControlPlane.Spec = old.Spec
		aliyunControlPlane.ObjectMeta.Finalizers = old.ObjectMeta.Finalizers
		if deferr := r.Client.Update(ctx, aliyunControlPlane, &client.UpdateOptions{}); deferr != nil {
			err = deferr
			return
		}
		log.Info("AliyunManagedControlPlane status update success")
	}()

	if !aliyunControlPlane.ObjectMeta.DeletionTimestamp.IsZero() {
		// todo: 处理删除流程
		return r.reconcileDelete(ctx, log, cluster, aliyunControlPlane)
	}

	log.Info("Reconciling AliyunManagedControlPlane")

	// 正常创建流程
	if cluster.Spec.InfrastructureRef.Kind != aliyunControlPlane.Kind {
		// Wait for the cluster infrastructure to be ready before creating machines

		// InfrastructureReady 由 cluster-api 项目中 reconcileInfrastructure() 函数更新,
		// 不过好像是取自 AliyunManagedCluster.status.ready 字段.
		// 即, 在 AliyunManagedCluster controller 设置 .status.ready = true 后,
		// 需要 cluster-api 先确认一遍, 才能继续往下进行.
		if !cluster.Status.InfrastructureReady {
			log.Info("Cluster infrastructure is not ready yet")
			return ctrl.Result{RequeueAfter: time.Second * 20}, nil
		}
	}

	if controllerutil.AddFinalizer(aliyunControlPlane, controlplanev1beta2.ManagedControlPlaneFinalizer) {
		if err := r.Client.Update(ctx, aliyunControlPlane, &client.UpdateOptions{}); err != nil {
			return ctrl.Result{}, err
		}
	}

	providerConfig, err := r.reconcileProviderConfig(ctx, log, aliyunControlPlane)
	if err != nil {
		conditions.MarkFalse(
			aliyunControlPlane,
			controlplanev1beta2.ProviderConfigReconcileReadyCondition,
			controlplanev1beta2.ProviderConfigReconcileFailedReason,
			clusterv1.ConditionSeverityError,
			err.Error(),
		)
		return ctrl.Result{}, errors.Wrapf(
			err, "failed to reconcile ProviderConfig for AliyunManagedControlPlane %s/%s",
			aliyunControlPlane.Namespace, aliyunControlPlane.Name,
		)
	}
	conditions.MarkTrue(aliyunControlPlane, controlplanev1beta2.ProviderConfigReconcileReadyCondition)

	vpcID, err := common.ReconcileVPC(
		ctx, log, r.Client,
		aliyunControlPlane, aliyunControlPlane.Spec.ClusterName,
		&aliyunControlPlane.Spec.Network.Vpc,
		xpv1.ResourceSpec{
			ProviderConfigReference: &xpv1.Reference{
				Name: providerConfig.Name,
			},
		},
	)
	if err != nil {
		conditions.MarkFalse(
			aliyunControlPlane,
			controlplanev1beta2.VPCReconcileReadyCondition,
			controlplanev1beta2.VPCReconcileFailedReason,
			clusterv1.ConditionSeverityError,
			err.Error(),
		)
		return ctrl.Result{}, errors.Wrapf(
			err, "failed to reconcile VPC for AliyunManagedControlPlane %s/%s",
			aliyunControlPlane.Namespace, aliyunControlPlane.Name,
		)
	}
	conditions.MarkTrue(aliyunControlPlane, controlplanev1beta2.VPCReconcileReadyCondition)

	vswitchIDs, err := common.ReconcileVSwitch(
		ctx, log, r.Client,
		aliyunControlPlane, aliyunControlPlane.Spec.ClusterName,
		xpv1.ResourceSpec{
			ProviderConfigReference: &xpv1.Reference{
				Name: providerConfig.Name,
			},
		},
		&aliyunControlPlane.Spec.Network.VSwitches,
		vpcID,
	)
	if err != nil {
		conditions.MarkFalse(
			aliyunControlPlane,
			controlplanev1beta2.VSwitchReconcileReadyCondition,
			controlplanev1beta2.VSwitchReconcileFailedReason,
			clusterv1.ConditionSeverityError,
			err.Error(),
		)
		return ctrl.Result{}, errors.Wrapf(
			err, "failed to reconcile VSwitch for AliyunManagedControlPlane %s/%s",
			aliyunControlPlane.Namespace, aliyunControlPlane.Name,
		)
	}
	conditions.MarkTrue(aliyunControlPlane, controlplanev1beta2.VSwitchReconcileReadyCondition)

	managedKubernetes := &csv1alpha1.ManagedKubernetes{}
	err = r.reconcileManagedKubernetes(
		ctx, log, cluster, aliyunControlPlane, managedKubernetes,
		providerConfig, vswitchIDs,
	)
	if err != nil {
		conditions.MarkFalse(
			aliyunControlPlane,
			controlplanev1beta2.ManagedKubernetesReconcileReadyCondition,
			controlplanev1beta2.ManagedKubernetesReconcileFailedReason,
			clusterv1.ConditionSeverityError,
			err.Error(),
		)
		return ctrl.Result{}, errors.Wrapf(
			err, "failed to reconcile ManagedKubernetes for AliyunManagedControlPlane %s/%s",
			aliyunControlPlane.Namespace, aliyunControlPlane.Name,
		)
	}

	r.setStatus(ctx, log, aliyunControlPlane, managedKubernetes)
	if aliyunControlPlane.Status.Ready && aliyunControlPlane.Spec.ControlPlaneEndpoint.IsValid() {
		err = r.reconcileKubeconfig(ctx, cluster, aliyunControlPlane, managedKubernetes)
		if err != nil {
			log.Error(err, "reconcile kubeconfig failed")
		}
	}
	log.V(5).Info("Successfully reconciled AliyunManagedMachinePool")

	return ctrl.Result{}, nil
}

func (r *AliyunManagedControlPlaneReconciler) reconcileManagedKubernetes(
	ctx context.Context,
	log logr.Logger,
	cluster *clusterv1.Cluster,
	aliyunControlPlane *controlplanev1beta2.AliyunManagedControlPlane,
	managedKubernetes *csv1alpha1.ManagedKubernetes,
	providerConfig *alibabacloudv1beta1.ProviderConfig,
	vswitchIDs []*string,
) (err error) {
	managedKubernetesKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}

	var exist bool
	if err := r.Client.Get(ctx, managedKubernetesKey, managedKubernetes); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		log.Info("ManagedKubernetes not found, try to create")
	} else {
		exist = true
	}

	if !exist {
		managedKubernetes = &csv1alpha1.ManagedKubernetes{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
				Labels:    map[string]string{},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         aliyunControlPlane.TypeMeta.GroupVersionKind().GroupVersion().String(),
						Kind:               aliyunControlPlane.TypeMeta.Kind,
						Name:               aliyunControlPlane.Name,
						UID:                aliyunControlPlane.UID,
						Controller:         ptr.To[bool](true),
						BlockOwnerDeletion: ptr.To[bool](true),
					},
				},
			},
			Spec: csv1alpha1.ManagedKubernetesSpec{
				ResourceSpec: xpv1.ResourceSpec{
					ProviderConfigReference: &xpv1.Reference{
						Name: providerConfig.Name,
					},
				},
				ForProvider: csv1alpha1.ManagedKubernetesParameters{
					Name:        &aliyunControlPlane.Spec.ClusterName,
					ClusterSpec: &aliyunControlPlane.Spec.ClusterSpec,
					Version:     &aliyunControlPlane.Spec.Version,

					ClusterDomain:    &aliyunControlPlane.Spec.ClusterDomain, // upjet 字段赋值时可为空, 空值时默认使用 cluster.local
					ServiceCidr:      &aliyunControlPlane.Spec.Network.ServiceCIDR,
					PodCidr:          &aliyunControlPlane.Spec.Network.PodCIDR,
					NewNATGateway:    &aliyunControlPlane.Spec.Network.NatGateway,
					WorkerVswitchIds: vswitchIDs,
					SecurityGroupID:  &aliyunControlPlane.Spec.Network.SecurityGroup.ID,

					Addons:    aliyunControlPlane.Spec.Addons,
					ProxyMode: &aliyunControlPlane.Spec.KubeProxy.ProxyMode,

					Timezone:           &aliyunControlPlane.Spec.TimeZone,
					Tags:               aliyunControlPlane.Spec.AdditionalTags,
					ResourceGroupID:    &aliyunControlPlane.Spec.ResourceGroup,
					DeletionProtection: &aliyunControlPlane.Spec.DeletionProtection,

					// todo:
					// Public: aliyunControlPlane.Spec.Network.EndpointAccess.Public,
				},
			},
		}
		if err := r.Client.Create(ctx, managedKubernetes, &client.CreateOptions{}); err != nil {
			return fmt.Errorf("ManagedKubernetes create failed: %s", err)
		}
		conditions.MarkTrue(aliyunControlPlane, controlplanev1beta2.AliyunManagedControlPlaneCreatingCondition)
		log.Info("ManagedKubernetes create success")
	} else {
		// todo: 可更新的字段
		managedKubernetes.Spec.ForProvider.Name = &aliyunControlPlane.Spec.ClusterName
		managedKubernetes.Spec.ForProvider.Version = &aliyunControlPlane.Spec.Version
		managedKubernetes.Spec.ForProvider.DeletionProtection = &aliyunControlPlane.Spec.DeletionProtection
		managedKubernetes.Spec.ForProvider.Addons = aliyunControlPlane.Spec.Addons
		managedKubernetes.Spec.ForProvider.Tags = aliyunControlPlane.Spec.AdditionalTags
		if aliyunControlPlane.Spec.Logging.Enable {
			managedKubernetes.Spec.ForProvider.ControlPlaneLogComponents = aliyunControlPlane.Spec.Logging.Components
			managedKubernetes.Spec.ForProvider.ControlPlaneLogTTL = &aliyunControlPlane.Spec.Logging.TTL
		} else {
			managedKubernetes.Spec.ForProvider.ControlPlaneLogComponents = []*string{}
			managedKubernetes.Spec.ForProvider.ControlPlaneLogTTL = nil
		}
		if err := r.Client.Update(ctx, managedKubernetes, &client.UpdateOptions{}); err != nil {
			return fmt.Errorf("ManagedKubernetes update failed: %s", err)
		}
		// todo: no need to Update
		if !conditions.IsTrue(aliyunControlPlane, controlplanev1beta2.AliyunManagedControlPlaneCreatingCondition) {
			conditions.MarkTrue(aliyunControlPlane, controlplanev1beta2.AliyunManagedControlPlaneUpdatingCondition)
		}
		log.Info("ManagedKubernetes update success")
	}
	conditions.MarkTrue(aliyunControlPlane, controlplanev1beta2.ManagedKubernetesReconcileReadyCondition)
	return
}

// setStatus 从 ManagedKubernetes 资源中查询并获取 endpoint 信息.
func (r *AliyunManagedControlPlaneReconciler) setStatus(
	_ context.Context,
	log logr.Logger,
	aliyunControlPlane *controlplanev1beta2.AliyunManagedControlPlane,
	managedKubernetes *csv1alpha1.ManagedKubernetes,
) (err error) {
	aliyunControlPlane.Status.Ready = false

	{
		// 根据 crossplane 资源状态设置 condition, 但对 control plane 的就绪状态并没有决定性影响
		var syncedCondition = managedKubernetes.GetCondition(xpv1.TypeSynced)
		if syncedCondition.Message != "" {
			conditions.MarkFalse(
				aliyunControlPlane,
				csv1alpha1.CrossPlaneReadyCondition,
				csv1alpha1.CrossPlaneFailedCondition,
				clusterv1.ConditionSeverityError,
				syncedCondition.Message,
			)
			aliyunControlPlane.Status.FailureMessage = syncedCondition.Message
		} else {
			conditions.MarkTrue(aliyunControlPlane, csv1alpha1.CrossPlaneReadyCondition)
		}
		var asyncCondition = managedKubernetes.GetCondition(upjetresource.TypeAsyncOperation)
		if asyncCondition.Message != "" {
			conditions.MarkFalse(
				aliyunControlPlane,
				csv1alpha1.UpjetProviderReadyCondition,
				csv1alpha1.UpjetProviderFailedCondition,
				clusterv1.ConditionSeverityError,
				asyncCondition.Message,
			)
			aliyunControlPlane.Status.FailureMessage = asyncCondition.Message
		} else {
			conditions.MarkTrue(aliyunControlPlane, csv1alpha1.UpjetProviderReadyCondition)
		}
		var lastAsyncCondition = managedKubernetes.GetCondition(upjetresource.TypeLastAsyncOperation)
		if lastAsyncCondition.Message != "" {
			conditions.MarkFalse(
				aliyunControlPlane,
				csv1alpha1.UpjetProviderReadyCondition,
				csv1alpha1.UpjetProviderFailedCondition,
				clusterv1.ConditionSeverityError,
				lastAsyncCondition.Message,
			)
			// aliyunControlPlane.Status.FailureMessage = lastAsyncCondition.Message
		} else {
			conditions.MarkTrue(aliyunControlPlane, csv1alpha1.UpjetProviderReadyCondition)
		}
	}

	var readyCondition = managedKubernetes.GetCondition(xpv1.TypeReady)
	if readyCondition.Status != corev1.ConditionTrue {
		// TypeReady != True 时, 可能是 Creating/Deleting 状态(Updating时一定也是 Ready 的)
		// todo: 设置失败原因
		return
	}

	log.Info("ManagedKubernetes is ready")

	// ready 之后, 移除 creating/updating 状态
	if conditions.IsTrue(aliyunControlPlane, controlplanev1beta2.AliyunManagedControlPlaneCreatingCondition) {
		conditions.MarkFalse(
			aliyunControlPlane,
			controlplanev1beta2.AliyunManagedControlPlaneCreatingCondition,
			"created", clusterv1.ConditionSeverityInfo, "",
		)
	}
	if conditions.IsTrue(aliyunControlPlane, controlplanev1beta2.AliyunManagedControlPlaneUpdatingCondition) {
		conditions.MarkFalse(
			aliyunControlPlane,
			controlplanev1beta2.AliyunManagedControlPlaneUpdatingCondition,
			"updated", clusterv1.ConditionSeverityInfo, "",
		)
	}

	endpointInfoObj := &csv1alpha1.ManagedKubernetesStatusConnection{}
	if err = endpointInfoObj.Decode(managedKubernetes.Status.AtProvider.Connections); err != nil {
		return
	}

	aliyunControlPlane.Status.Ready = true
	aliyunControlPlane.Status.FailureMessage = ""
	aliyunControlPlane.Status.ClusterID = *managedKubernetes.Status.AtProvider.ID
	aliyunControlPlane.Status.Version = *managedKubernetes.Status.AtProvider.Version
	aliyunControlPlane.Status.NetworkStatus = controlplanev1beta2.AliyunManagedControlPlaneStatusNetwork{
		SecurityGroup:  *managedKubernetes.Status.AtProvider.SecurityGroupID,
		ApiServerSlbID: *managedKubernetes.Status.AtProvider.SlbID,
		NatGatewayID:   "", // todo
	}
	aliyunControlPlane.Status.Addons = managedKubernetes.Status.AtProvider.Addons

	// spec 部分
	aliyunControlPlane.Spec.ControlPlaneEndpoint = clusterv1.APIEndpoint{
		Host: endpointInfoObj.Host, Port: int32(endpointInfoObj.Port),
	}
	if aliyunControlPlane.Spec.Network.Vpc.Name != "" {
		aliyunControlPlane.Spec.Network.Vpc.ResourceID = *managedKubernetes.Status.AtProvider.VPCID
	}

	conditions.MarkTrue(aliyunControlPlane, controlplanev1beta2.AliyunManagedControlPlaneReadyCondition)

	return
}

func (r *AliyunManagedControlPlaneReconciler) reconcileDelete(
	ctx context.Context,
	log logr.Logger,
	cluster *clusterv1.Cluster,
	controlPlane *controlplanev1beta2.AliyunManagedControlPlane,
) (_ ctrl.Result, err error) {
	log = log.WithValues("operation", "delete")

	managedKubernetes := &csv1alpha1.ManagedKubernetes{}
	managedKubernetesKey := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if err := r.Client.Get(ctx, managedKubernetesKey, managedKubernetes); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	} else {
		log = log.WithValues("ManagedKubernetes", managedKubernetes.Name)
		// 判断是否正在删除中
		if !managedKubernetes.ObjectMeta.DeletionTimestamp.IsZero() {
			log.Info("ManagedKubernetes deleting")
			return ctrl.Result{RequeueAfter: time.Second * 20}, nil
		}

		if err := r.Client.Delete(ctx, managedKubernetes); err != nil {
			return ctrl.Result{RequeueAfter: time.Second * 20}, err
		}
		log.Info("ManagedKubernetes delete success")
		return ctrl.Result{}, nil
	}
	log.Info("ManagedKubernetes doesn't exist now")

	err = common.ReconcileRemoveVSwitch(
		ctx, log, r.Client, &controlPlane.Spec.Network.VSwitches, r.DeleteTimeout,
	)
	if err != nil {
		for _, e := range err.(utilerrors.Aggregate).Errors() {
			if e == common.ErrorTimeout {
				continue
			}
			return ctrl.Result{}, err
		}
	}
	log.Info("VSwitches doesn't exist now")

	err = common.ReconcileRemoveVPC(
		ctx, log, r.Client, &controlPlane.Spec.Network.Vpc, r.DeleteTimeout,
	)
	if err != nil {
		if err != common.ErrorTimeout {
			return ctrl.Result{}, err
		}
	}
	log.Info("VPC doesn't exist now")

	// 待清理完关联资源后, 移除 finalizer
	controllerutil.RemoveFinalizer(controlPlane, controlplanev1beta2.ManagedControlPlaneFinalizer)
	log.Info("AliyunManagedControlPlane remove finalizer success")

	return
}

// SetupWithManager sets up the controller with the Manager.
func (r *AliyunManagedControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) (err error) {
	// 添加 Indexer 索引器, 通过 uid 信息找到 aliyunControlPlane 资源.
	if err = mgr.GetFieldIndexer().IndexField(ctx,
		&controlplanev1beta2.AliyunManagedControlPlane{},
		common.IndexAliyunControlPlaneByUID,
		r.indexAliyunControlPlaneByUID,
	); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1beta2.AliyunManagedControlPlane{}).
		// 由于 ManagedKubernetes 本身是 cluster scope 的, 其 ownerRef 中也就不包含 namespace 信息了,
		// 因此ta的变动将无法被正确地映射成 Request.NamespacedName 信息(namespace值将为""), 无法查询到.
		// KubernetesNodePool 同理.
		// Owns(&csv1alpha1.ManagedKubernetes{}).
		Watches(
			// 在 ManagedKubernetes 状态更新时, 及时触发到 aliyunControlPlane 对象.
			&csv1alpha1.ManagedKubernetes{},
			handler.EnqueueRequestsFromMapFunc(
				r.mapManagedKubernetesToAliyunControlPlane(ctx),
			),
		).
		Complete(r)
}
