package infrastructure

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"strings"
	"time"

	alibabacloudv1beta1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/alibabacloud/v1beta1"
	ecsv1alpha1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/ecs/v1alpha1"
	infrav1beta2 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/infrastructure/v1beta2"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/clients"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/controller/controlplane"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	AliyunMachineFinalizer = "infrastructure.cluster.x-k8s.io/aliyunmachine" // 如已在 api 包里定义，可复用那处常量

	// 用 label 把 cluster-scoped 的 Upjet Instance 与 namespaced 的 AliyunMachine 关联
	labelOwnedByAMNS   = "infra.alibabacloud.com-owned-by-am-ns"
	labelOwnedByAMName = "infra.alibabacloud.com-owned-by-am-name"
)

// ---------- RBAC ----------

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunmachines/finalizers,verbs=update

//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines,verbs=get;list;watch

//+kubebuilder:rbac:groups=ecs.alibabacloud.crossplane.io,resources=instances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ecs.alibabacloud.crossplane.io,resources=instances/status,verbs=get;update;patch

type AliyunMachineReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CredentialSecret *controlplane.CredentialSecret
}

func (r *AliyunMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("AliyunMachine", req.NamespacedName)

	am := &infrav1beta2.AliyunMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, am); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 删除流程
	if !am.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, log, am)
	}

	// 确保 finalizer
	if controllerAdded := ensureFinalizer(am, AliyunMachineFinalizer); controllerAdded {
		if err := r.Client.Update(ctx, am); err != nil {
			return ctrl.Result{}, err
		}
	}

	// ProviderConfig & creds Secret
	pc, err := r.reconcileProviderConfig(ctx, log, am)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 获取对应 controllerplan 对应的 userData
	userDataB64, err := r.getBootstrapUserDataBase64(ctx, am)
	if err != nil {
		// 没有 bootstrap secret：等待
		conditions.MarkFalse(am, clusterv1.BootstrapReadyCondition,
			"WaitingForBootstrapData", clusterv1.ConditionSeverityInfo, err.Error())
		_ = r.Client.Status().Update(ctx, am)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	instName := fmt.Sprintf("%s-%s", am.Name, strings.ToLower(string(am.UID))[:8])
	want := r.buildDesiredInstance(am, pc.Name, userDataB64, instName)
	cur := &ecsv1alpha1.Instance{}

	if err := r.Client.Get(ctx, types.NamespacedName{Name: instName}, cur); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		// Create
		if err := r.Client.Create(ctx, want); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Created ECS Instance", "name", instName)
	} else {
		// 仅在“软字段”变化时 patch
		if !equalInstanceForProviderSoft(cur.Spec.ForProvider, want.Spec.ForProvider) {
			// 这里只更新可以更改的参数
			mergeSoftFields(&cur.Spec.ForProvider, &want.Spec.ForProvider)

			if err := r.Client.Patch(ctx, cur, client.MergeFrom(cur.DeepCopy())); err != nil {
				return ctrl.Result{}, err
			}
			log.Info("Patched ECS Instance (soft fields)", "name", instName)
		}
		// 记录无法更新的字段
		if hasHardImmutableDiff(cur.Spec.ForProvider, want.Spec.ForProvider) {
			log.Info("Immutable diff detected; skip in-place update, use template rollout", "name", instName)
		}
	}

	inst := &ecsv1alpha1.Instance{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: instName}, inst); err != nil {
		return ctrl.Result{}, err
	}

	if inst.Status.AtProvider.ID == nil {
		conditions.MarkFalse(am, clusterv1.InfrastructureReadyCondition, "InstanceCreating", clusterv1.ConditionSeverityInfo, "waiting for ECS instance ID")
		_ = r.Client.Status().Update(ctx, am)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// 等到实例进入 Running（Upjet: InstanceObservation.Status -> "Running"/"Stopped"...）
	if inst.Status.AtProvider.Status == nil || *inst.Status.AtProvider.Status != "Running" {
		conditions.MarkFalse(am, clusterv1.InfrastructureReadyCondition,
			"InstanceNotRunning", clusterv1.ConditionSeverityInfo,
			fmt.Sprintf("instance status is %v", inst.Status.AtProvider.Status))
		_ = r.Client.Status().Update(ctx, am)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	providerID := fmt.Sprintf("alicloud://%s", *inst.Status.AtProvider.ID)
	if am.Spec.ProviderID != providerID || am.Status.ProviderID != providerID {
		am.Spec.ProviderID = providerID
	}

	// 更新 spec/metadata（比如 Finalizers、Spec 字段）
	if err := r.Client.Update(ctx, am); err != nil {
		return ctrl.Result{}, err
	}

	var addrs []clusterv1.MachineAddress
	if v := inst.Status.AtProvider.PrimaryIPAddress; v != nil && *v != "" {
		addrs = append(addrs, clusterv1.MachineAddress{Type: clusterv1.MachineInternalIP, Address: *v})
	}
	if v := inst.Status.AtProvider.PublicIP; v != nil && *v != "" {
		addrs = append(addrs, clusterv1.MachineAddress{Type: clusterv1.MachineExternalIP, Address: *v})
	}
	if len(addrs) > 0 {
		am.Status.Addresses = addrs
	}

	am.Status.Addresses = collectAddresses(inst)
	am.Status.ProviderID = providerID

	// 标记就绪
	conditions.MarkTrue(am, clusterv1.InfrastructureReadyCondition)
	conditions.MarkTrue(am, clusterv1.ReadyCondition)
	conditions.MarkTrue(am, clusterv1.BootstrapReadyCondition)

	if err := r.Client.Status().Update(ctx, am); err != nil {
		return ctrl.Result{}, err
	}

	// 快速的使 machine ready
	if owner, err := util.GetOwnerMachine(ctx, r.Client, am.ObjectMeta); err == nil && owner != nil {
		if owner.Spec.ProviderID == nil || *owner.Spec.ProviderID != providerID {
			ob := owner.DeepCopy()
			owner.Spec.ProviderID = &providerID
			_ = r.Client.Patch(ctx, owner, client.MergeFrom(ob))
		}
	}
	return ctrl.Result{}, nil
}

func (r *AliyunMachineReconciler) reconcileDelete(ctx context.Context, log logr.Logger, am *infrav1beta2.AliyunMachine) (ctrl.Result, error) {
	instName := fmt.Sprintf("%s-%s", am.Name, strings.ToLower(string(am.UID))[:8])

	// 删除 Upjet Instance
	inst := &ecsv1alpha1.Instance{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: instName}, inst); err == nil {
		if err := r.Client.Delete(ctx, inst); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// 移除 finalizer
	removeFinalizer(am, AliyunMachineFinalizer)
	if err := r.Client.Update(ctx, am); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("AliyunMachine delete complete")
	return ctrl.Result{}, nil
}

func (r *AliyunMachineReconciler) reconcileProviderConfig(
	ctx context.Context,
	log logr.Logger,
	am *infrav1beta2.AliyunMachine,
) (*alibabacloudv1beta1.ProviderConfig, error) {

	region := am.Spec.RegionID
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

func (r *AliyunMachineReconciler) getBootstrapUserDataBase64(ctx context.Context, am *infrav1beta2.AliyunMachine) (string, error) {
	// 找 owner Machine
	var ownerMachineName string
	for _, ref := range am.OwnerReferences {
		if ref.Kind == "Machine" && ref.APIVersion == clusterv1.GroupVersion.String() {
			ownerMachineName = ref.Name
			break
		}
	}
	if ownerMachineName == "" {
		return "", fmt.Errorf("owner Machine not found in ownerReferences")
	}
	m := &clusterv1.Machine{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: am.Namespace, Name: ownerMachineName}, m); err != nil {
		return "", err
	}
	if m.Spec.Bootstrap.DataSecretName == nil {
		return "", fmt.Errorf("bootstrap data secret name is nil")
	}
	sec := &corev1.Secret{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: am.Namespace, Name: *m.Spec.Bootstrap.DataSecretName}, sec); err != nil {
		return "", err
	}
	// RKE2 bootstrap 的 key 一般为 "value"
	raw := sec.Data["value"]
	if len(raw) == 0 {
		return "", fmt.Errorf("bootstrap secret %q has empty 'value'", *m.Spec.Bootstrap.DataSecretName)
	}
	// Upjet 支持 base64，推荐传 base64；这里统一编码
	return base64.StdEncoding.EncodeToString(raw), nil
}

func (r *AliyunMachineReconciler) buildDesiredInstance(am *infrav1beta2.AliyunMachine, providerConfigName, userDataB64, instName string) *ecsv1alpha1.Instance {
	fp := ecsv1alpha1.InstanceParameters{
		InstanceName:            &instName,
		InstanceType:            &am.Spec.InstanceType,
		VswitchID:               &am.Spec.VSwitchID,
		SecurityGroups:          toPtrSlice(am.Spec.SecurityGroupIDs),
		PrivateIP:               am.Spec.PrivateIP,
		ImageID:                 am.Spec.ImageID,
		KeyName:                 am.Spec.KeyName,
		UserData:                &userDataB64, // Upjet 支持 base64，推荐
		InternetMaxBandwidthOut: toFloat64Ptr(am.Spec.InternetMaxBandwidthOut),
		InternetMaxBandwidthIn:  toFloat64Ptr(am.Spec.InternetMaxBandwidthIn),
		InternetChargeType:      am.Spec.InternetChargeType,
		InstanceChargeType:      am.Spec.InstanceChargeType,
		Tags:                    toPtrMap(am.Spec.Tags),
		VolumeTags:              toPtrMap(am.Spec.VolumeTags),
	}

	// 系统盘映射
	if sd := am.Spec.SystemDisk; sd != nil {
		fp.SystemDiskCategory = sd.Category
		fp.SystemDiskPerformanceLevel = sd.PerformanceLevel
		fp.SystemDiskSize = toFloat64Ptr(sd.Size)
		fp.SystemDiskEncrypted = sd.Encrypted
		fp.SystemDiskEncryptAlgorithm = sd.EncryptAlgorithm
		fp.SystemDiskKMSKeyID = sd.KMSKeyID
		fp.SystemDiskAutoSnapshotPolicyID = sd.AutoSnapshotPolID
		fp.SystemDiskName = sd.Name
		fp.SystemDiskDescription = sd.Description
	}

	// 数据盘映射
	if len(am.Spec.DataDisks) > 0 {
		fp.DataDisks = make([]ecsv1alpha1.DataDisksParameters, 0, len(am.Spec.DataDisks))
		for _, d := range am.Spec.DataDisks {
			fp.DataDisks = append(fp.DataDisks, ecsv1alpha1.DataDisksParameters{
				Category:           d.Category,
				PerformanceLevel:   d.PerformanceLevel,
				Size:               toFloat64Ptr(d.Size),
				SnapshotID:         d.SnapshotID,
				DeleteWithInstance: d.DeleteWithInstance,
				KMSKeyID:           d.KMSKeyID,
				Encrypted:          d.Encrypted,
				Name:               d.Name,
				Description:        d.Description,
				Device:             d.Device,
			})
		}
	}

	labels := map[string]string{
		labelOwnedByAMNS:   am.Namespace,
		labelOwnedByAMName: am.Name,
	}

	return &ecsv1alpha1.Instance{
		ObjectMeta: meta.ObjectMeta{
			Name:   instName,
			Labels: labels,
		},
		Spec: ecsv1alpha1.InstanceSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: providerConfigName},
			},
			ForProvider: fp,
		},
	}
}

// Machine -> AliyunMachine 的映射：如果 Machine.spec.infrastructureRef 指向某个 AliyunMachine，就把它入队
func (r *AliyunMachineReconciler) machineToAliyunMachine(ctx context.Context, obj client.Object) []ctrl.Request {
	m, ok := obj.(*clusterv1.Machine)
	if !ok {
		return nil
	}
	if m.Spec.InfrastructureRef.Kind != "AliyunMachine" ||
		m.Spec.InfrastructureRef.APIVersion != "infrastructure.cluster.x-k8s.io/v1beta2" ||
		m.Spec.InfrastructureRef.Name == "" {
		return nil
	}
	// Machine 与 InfraMachine 在同 namespace（CAPI 约定）
	return []ctrl.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: m.Namespace,
			Name:      m.Spec.InfrastructureRef.Name,
		},
	}}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AliyunMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1beta2.AliyunMachine{}).
		// 当 Machine 更新（尤其是 .spec.bootstrap.dataSecretName 出现）时，反向触发对应的 AliyunMachine
		Watches(&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(r.machineToAliyunMachine)).
		Named("infrastructure-aliyunmachine").
		Complete(r)
}
