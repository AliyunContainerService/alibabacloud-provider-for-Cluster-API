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
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	infrav1beta2 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/infrastructure/v1beta2"
	nlbv1alpha1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/nlb/v1alpha1"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/clients"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/internal/controller/controlplane"
	nlbclient "github.com/alibabacloud-go/nlb-20220430/v4/client"
	"github.com/alibabacloud-go/tea/dara"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// AliyunClusterReconciler reconciles a AliyunCluster object
type AliyunClusterReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	CredentialSecret *controlplane.CredentialSecret
}

const (
	AliyunClusterFinalizer = "aliyuncluster.infrastructure.cluster.x-k8s.io/finalizer"
	labelOwnedByACNS       = "infra.alibabacloud.com-owned-by-ac-ns"
	labelOwnedByACName     = "infra.alibabacloud.com-owned-by-ac-name"
	labelSpecHash          = "infra.alibabacloud.com-spec-hash"
)

type desiredChild struct {
	Key             string
	SpecHash        string
	Protocol        string
	Port            string
	LoadBalancerID  string
	VpcID           string
	ListenerName    string
	ServerGroupName string
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=aliyunclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
func (r *AliyunClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("AliyunCluster", req.NamespacedName.String())
	ac := &infrav1beta2.AliyunCluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, ac); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	cluster, err := util.GetOwnerCluster(ctx, r.Client, ac.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		conditions.MarkFalse(ac, clusterv1.InfrastructureReadyCondition, "OwnerClusterNotFound", clusterv1.ConditionSeverityInfo, "waiting for owner Cluster")
		_ = r.Client.Status().Update(ctx, ac)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	// 删除
	if !ac.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, log, ac)
	}
	// 添加 finalizer
	if controllerutil.AddFinalizer(ac, AliyunClusterFinalizer) {
		if err := r.Client.Update(ctx, ac); err != nil {
			return ctrl.Result{}, err
		}
	}
	// ProviderConfig & creds Secret
	pcName, err := r.reconcileProviderConfig(ctx, log, ac)
	if err != nil {
		return ctrl.Result{}, err
	}
	// 创建 aliyun NLB
	// 如果 NLB 已经创建了直接返回
	nlbName := fmt.Sprintf("%s-cp-nlb-%s", ac.Name, strings.ToLower(string(ac.UID))[:8])
	if ac.Spec.ExistingNLBDNS == "" || ac.Spec.ExistingNLBID == "" {
		want := r.buildNLB(ac, nlbName, pcName)
		cur := &nlbv1alpha1.LoadBalancer{}

		if err := r.Client.Get(ctx, types.NamespacedName{Name: nlbName}, cur); err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			// Create
			if err := r.Client.Create(ctx, want); err != nil {
				return ctrl.Result{}, err
			}
			log.Info("Created ECS Instance", "name", nlbName)
		} else {
			if !equalNlbForProviderSoft(cur.Spec.ForProvider, want.Spec.ForProvider) {
				orig := cur.DeepCopy()
				// 更新 zoneMappings
				cur.Spec.ForProvider.ZoneMappings = want.Spec.ForProvider.ZoneMappings

				if err := r.Client.Patch(ctx, cur, client.MergeFrom(orig.DeepCopy())); err != nil {
					return ctrl.Result{}, err
				}
				log.Info("Patched NLB (soft fields)", "name", nlbName)
			}
		}
	}
	nlb := &nlbv1alpha1.LoadBalancer{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: nlbName}, nlb); err != nil {
		return ctrl.Result{}, err
	}
	if nlb.Status.AtProvider.ID == nil || str(nlb.Status.AtProvider.Status) != "Active" {
		conditions.MarkFalse(ac, clusterv1.InfrastructureReadyCondition, "NLBCreating", clusterv1.ConditionSeverityInfo, "waiting for NLB ID")
		_ = r.Client.Status().Update(ctx, ac)
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}

	// nlb.Status.AtProvider.DNSName 并不能获得只能通过 SDK 获得不知道什么情况，当这个 nlb active 之后用 sdk 重新获得 DNS
	nlbID := str(nlb.Status.AtProvider.ID)
	region := ac.Spec.RegionID
	sdkClient, err := clients.CreateNlbSDKClient(region)
	if err != nil {
		return ctrl.Result{}, err
	}
	getLoadBalancerAttributeRequest := &nlbclient.GetLoadBalancerAttributeRequest{
		RegionId:       dara.String(region),
		LoadBalancerId: dara.String(nlbID),
	}
	runtime := &dara.RuntimeOptions{}
	resp, err := sdkClient.GetLoadBalancerAttributeWithOptions(getLoadBalancerAttributeRequest, runtime)
	if err != nil {
		return ctrl.Result{}, err
	}
	// 获得 nlbDNS
	nlbDNS := str(resp.GetBody().DNSName)
	res, err := r.reconcileNLBServerGroupsAndListeners(ctx, log, ac, pcName, nlbID)
	if err != nil {
		return res, err
	}
	if res.Requeue || res.RequeueAfter > 0 {
		return res, nil
	}
	// 更新 spec
	needSpecUpdate := false
	if ac.Spec.ExistingNLBID != nlbID {
		ac.Spec.ExistingNLBID = nlbID
		needSpecUpdate = true
	}
	if ac.Spec.ExistingNLBDNS != nlbDNS {
		ac.Spec.ExistingNLBID = nlbDNS
		needSpecUpdate = true
	}
	if needSpecUpdate {
		if err := r.Client.Update(ctx, ac); err != nil {
			return ctrl.Result{}, err
		}
	}
	// 更新 ac status
	sgList := &nlbv1alpha1.ServerGroupList{}
	if err := r.Client.List(ctx, sgList,
		client.MatchingLabels{
			labelOwnedByACNS:   ac.Namespace,
			labelOwnedByACName: ac.Name,
		},
	); err != nil {
		return ctrl.Result{}, err
	}
	var sgIDs []string
	for _, sg := range sgList.Items {
		if sg.Status.AtProvider.ID != nil && *sg.Status.AtProvider.ID != "" {
			sgIDs = append(sgIDs, *sg.Status.AtProvider.ID)
		}
	}
	needStatusUpdate := false
	if ac.Status.NlbID != nlbID {
		ac.Status.NlbID = nlbID
		needStatusUpdate = true
	}
	if ac.Status.NlbDNS != nlbDNS {
		ac.Status.NlbDNS = nlbDNS
		needStatusUpdate = true
	}
	if !stringSliceEqual(ac.Status.ServerGroupIDs, sgIDs) {
		ac.Status.ServerGroupIDs = sgIDs
		needStatusUpdate = true
	}
	conditions.MarkTrue(ac, clusterv1.InfrastructureReadyCondition)
	needStatusUpdate = true
	if needStatusUpdate {
		if err := r.Client.Status().Update(ctx, ac); err != nil {
			return ctrl.Result{}, err
		}
	}
	port := ac.Spec.Port
	if cluster != nil && nlbDNS != "" {
		want := clusterv1.APIEndpoint{Host: nlbDNS, Port: port}
		// 更新 cluster ControlPlaneEndpoint
		if cluster.Spec.ControlPlaneEndpoint.Host != want.Host || cluster.Spec.ControlPlaneEndpoint.Port != want.Port {
			orig := cluster.DeepCopy()
			cluster.Spec.ControlPlaneEndpoint = want
			if err := r.Client.Patch(ctx, cluster, client.MergeFrom(orig)); err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	// 设置好 dns 之后就可以把 ReadyCondition 设置为 true 了
	ac.Status.Ready = true
	conditions.MarkTrue(ac, clusterv1.ReadyCondition)
	if err := r.Client.Status().Update(ctx, ac); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AliyunClusterReconciler) reconcileProviderConfig(
	ctx context.Context,
	log logr.Logger,
	ac *infrav1beta2.AliyunCluster,
) (string, error) {
	region := ac.Spec.RegionID
	if region == "" && r.CredentialSecret.Region != "" {
		region = r.CredentialSecret.Region
	}
	if region == "" {
		return "", fmt.Errorf("region not set: add annotation %q or configure Reconciler.CredentialSecret.Region", RegionAnnotation)
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
			return "", err
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
			return "", err
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
			return "", err
		}
	}
	// ProviderConfig 用 region 作为名字
	if err := ensureUpjetProviderConfigNoStore(
		ctx, r.Client, "alibabacloud.alibabacloud.com", "v1beta1",
		region, region, secretKey.Namespace, secretKey.Name, r.CredentialSecret.Key,
	); err != nil {
		return "", err
	}
	return region, nil
}

func (r *AliyunClusterReconciler) reconcileDelete(
	ctx context.Context,
	log logr.Logger,
	ac *infrav1beta2.AliyunCluster,
) (ctrl.Result, error) {
	lbls := client.MatchingLabels(map[string]string{
		labelOwnedByACNS:   ac.Namespace,
		labelOwnedByACName: ac.Name,
	})
	var needRequeue bool
	// 删除 Listeners
	ll := &nlbv1alpha1.ListenerList{}
	if err := r.Client.List(ctx, ll, lbls); err != nil {
		return ctrl.Result{}, err
	}
	for i := range ll.Items {
		obj := &ll.Items[i]
		if err := r.Client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	if len(ll.Items) > 0 {
		needRequeue = true
	}
	// 删除 ServerGroups
	sgl := &nlbv1alpha1.ServerGroupList{}
	if err := r.Client.List(ctx, sgl, lbls); err != nil {
		return ctrl.Result{}, err
	}
	for i := range sgl.Items {
		obj := &sgl.Items[i]
		if err := r.Client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	if len(sgl.Items) > 0 {
		needRequeue = true
	}
	// 删除 NLB
	lbl := &nlbv1alpha1.LoadBalancerList{}
	if err := r.Client.List(ctx, lbl, lbls); err != nil {
		return ctrl.Result{}, err
	}
	for i := range lbl.Items {
		obj := &lbl.Items[i]
		if err := r.Client.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	if len(lbl.Items) > 0 {
		needRequeue = true
	}
	// Upjet 正在删 → 等彻底删掉再移除 finalizer
	if needRequeue {
		log.Info("Waiting for NLB resources to be deleted")
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	// 清理完 → 移除 finalizer
	controllerutil.RemoveFinalizer(ac, AliyunClusterFinalizer)
	if err := r.Client.Update(ctx, ac); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("AliyunCluster delete complete")
	return ctrl.Result{}, nil
}

func (r *AliyunClusterReconciler) buildNLB(ac *infrav1beta2.AliyunCluster, nlbName, providerConfigName string) *nlbv1alpha1.LoadBalancer {
	lp := nlbv1alpha1.LoadBalancerParameters{
		LoadBalancerName: &nlbName,
		AddressType:      &ac.Spec.AddressType,
		VPCID:            &ac.Spec.VpcId,
		ZoneMappings:     MapZoneMappingsToNLB(ac.Spec.ZoneMappings),
	}
	labels := map[string]string{
		labelOwnedByACNS:   ac.Namespace,
		labelOwnedByACName: ac.Name,
	}
	return &nlbv1alpha1.LoadBalancer{
		ObjectMeta: meta.ObjectMeta{
			Name:   nlbName,
			Labels: labels,
		},
		Spec: nlbv1alpha1.LoadBalancerSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: providerConfigName},
			},
			ForProvider: lp,
		},
	}
}

func MapZoneMappingsToNLB(specMappings []infrav1beta2.ZoneMappings) []nlbv1alpha1.ZoneMappingsParameters {
	if len(specMappings) == 0 {
		return nil
	}
	out := make([]nlbv1alpha1.ZoneMappingsParameters, 0, len(specMappings))
	for _, zm := range specMappings {
		out = append(out, nlbv1alpha1.ZoneMappingsParameters{
			VswitchID: &zm.VSwitchId,
			ZoneID:    &zm.ZoneId,
		})
	}
	return out
}

func (r *AliyunClusterReconciler) reconcileNLBServerGroupsAndListeners(
	ctx context.Context,
	log logr.Logger,
	ac *infrav1beta2.AliyunCluster,
	providerConfigName string,
	nlbID string,
) (ctrl.Result, error) {
	// 期望集合
	want := r.buildDesiredChildren(ac, nlbID)
	// 当前集合（通过 owner labels 过滤）现在已经创建出来的后端服务器组
	curSGs, err := r.listOwnedServerGroups(ctx, ac)
	if err != nil {
		return ctrl.Result{}, err
	}
	curLsns, err := r.listOwnedListeners(ctx, ac)
	if err != nil {
		return ctrl.Result{}, err
	}
	// 删除多余
	if err := r.deleteExtraneousListeners(ctx, log, ac, curLsns, want); err != nil {
		return ctrl.Result{}, err
	}
	// 同 listener 删除逻辑
	if err := r.deleteExtraneousServerGroups(ctx, log, ac, curSGs, want); err != nil {
		return ctrl.Result{}, err
	}
	// 确保 ServerGroup 存在并拿到 ID
	sgIDs := map[string]string{}
	needRequeue := false
	for _, d := range want {
		id, created, err := r.ensureServerGroup(ctx, log, ac, providerConfigName, d)
		if err != nil {
			return ctrl.Result{}, err
		}
		if id == "" {
			// Upjet 还没把 ID 写回，等下一轮再创建 Listener
			needRequeue = true
		}
		if created {
			log.Info("Created NLB ServerGroup", "name", d.ServerGroupName)
		}
		sgIDs[d.ServerGroupName] = id
	}
	// 确保 Listener 存在
	for _, d := range want {
		sgID := sgIDs[d.ServerGroupName]
		if sgID == "" {
			needRequeue = true
			continue
		}
		created, err := r.ensureListener(ctx, log, ac, providerConfigName, d, sgID)
		if err != nil {
			return ctrl.Result{}, err
		}
		if created {
			log.Info("Created NLB Listener", "name", d.ListenerName)
		}
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func (r *AliyunClusterReconciler) buildDesiredChildren(ac *infrav1beta2.AliyunCluster, lbID string) map[string]desiredChild {
	out := map[string]desiredChild{}
	for _, l := range ac.Spec.Listeners {
		proto := l.ListenerProtocol
		port := l.ListenerPort
		vpc := firstNonEmpty(l.VpcId, ac.Spec.VpcId)
		if proto == "" || port == "" || vpc == "" {
			continue
		}
		key := proto + ":" + port
		ln := fmt.Sprintf("lsn-%s-%s-al", strings.ToLower(proto), port)
		sgn := fmt.Sprintf("sg-%s-%s-al", strings.ToLower(proto), port)
		h := sha1.Sum([]byte(strings.Join([]string{proto, port, lbID, vpc}, "|")))
		// 判断是否有变化
		specHash := hex.EncodeToString(h[:])
		out[key] = desiredChild{
			Key:             key,
			SpecHash:        specHash,
			Protocol:        proto,
			Port:            port,
			LoadBalancerID:  lbID,
			VpcID:           vpc,
			ListenerName:    ln,
			ServerGroupName: sgn,
		}
	}
	return out
}

func (r *AliyunClusterReconciler) listOwnedServerGroups(ctx context.Context, ac *infrav1beta2.AliyunCluster) ([]nlbv1alpha1.ServerGroup, error) {
	var list nlbv1alpha1.ServerGroupList
	if err := r.Client.List(ctx, &list, client.MatchingLabels{
		labelOwnedByACNS:   ac.Namespace,
		labelOwnedByACName: ac.Name,
	}); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *AliyunClusterReconciler) listOwnedListeners(ctx context.Context, ac *infrav1beta2.AliyunCluster) ([]nlbv1alpha1.Listener, error) {
	var list nlbv1alpha1.ListenerList
	if err := r.Client.List(ctx, &list, client.MatchingLabels{
		labelOwnedByACNS:   ac.Namespace,
		labelOwnedByACName: ac.Name,
	}); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (r *AliyunClusterReconciler) deleteExtraneousListeners(
	ctx context.Context, log logr.Logger, ac *infrav1beta2.AliyunCluster,
	cur []nlbv1alpha1.Listener, want map[string]desiredChild,
) error {
	wantByName := map[string]desiredChild{}
	for _, d := range want {
		wantByName[d.ListenerName] = d
	}
	// 获取当前的删除 want 里没有的
	for i := range cur {
		d, ok := wantByName[cur[i].Name]
		if !ok {
			if err := r.Client.Delete(ctx, &cur[i]); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			log.Info("Deleted extraneous NLB Listener", "name", cur[i].Name)
			continue
		}
		// 规格 hash 不同删除，如果 name 一样但是 Hash 记录的值不同也删除重建
		if cur[i].Labels[labelSpecHash] != d.SpecHash {
			if err := r.Client.Delete(ctx, &cur[i]); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			log.Info("Deleted outdated NLB Listener (spec changed)", "name", cur[i].Name)
		}
	}
	return nil
}

func (r *AliyunClusterReconciler) deleteExtraneousServerGroups(
	ctx context.Context, log logr.Logger, ac *infrav1beta2.AliyunCluster,
	cur []nlbv1alpha1.ServerGroup, want map[string]desiredChild,
) error {
	wantByName := map[string]desiredChild{}
	for _, d := range want {
		wantByName[d.ServerGroupName] = d
	}
	for i := range cur {
		d, ok := wantByName[cur[i].Name]
		if !ok {
			if err := r.Client.Delete(ctx, &cur[i]); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			log.Info("Deleted extraneous NLB ServerGroup", "name", cur[i].Name)
			continue
		}
		if cur[i].Labels[labelSpecHash] != d.SpecHash {
			if err := r.Client.Delete(ctx, &cur[i]); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
			log.Info("Deleted outdated NLB ServerGroup (spec changed)", "name", cur[i].Name)
		}
	}
	return nil
}

func (r *AliyunClusterReconciler) ensureServerGroup(
	ctx context.Context,
	log logr.Logger,
	ac *infrav1beta2.AliyunCluster,
	providerConfigName string,
	d desiredChild,
) (id string, created bool, err error) {
	labels := map[string]string{
		labelOwnedByACNS:   ac.Namespace,
		labelOwnedByACName: ac.Name,
		labelSpecHash:      d.SpecHash,
	}
	cur := &nlbv1alpha1.ServerGroup{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: d.ServerGroupName}, cur); err != nil {
		if !apierrors.IsNotFound(err) {
			return "", false, err
		}
		sg := &nlbv1alpha1.ServerGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:   d.ServerGroupName,
				Labels: labels,
			},
			Spec: nlbv1alpha1.ServerGroupSpec{
				ResourceSpec: xpv1.ResourceSpec{
					ProviderConfigReference: &xpv1.Reference{Name: providerConfigName},
				},
				ForProvider: nlbv1alpha1.ServerGroupParameters{
					ServerGroupName: &d.ServerGroupName,
					VPCID:           &d.VpcID,
				},
			},
		}
		if err := r.Client.Create(ctx, sg); err != nil {
			return "", false, err
		}
		return "", true, nil
	}

	// 已存在：不更新，直接返回 ID
	if cur.Status.AtProvider.ID != nil {
		return *cur.Status.AtProvider.ID, false, nil
	}
	return "", false, nil
}

func (r *AliyunClusterReconciler) ensureListener(
	ctx context.Context,
	log logr.Logger,
	ac *infrav1beta2.AliyunCluster,
	providerConfigName string,
	d desiredChild,
	serverGroupID string,
) (created bool, err error) {
	labels := map[string]string{
		labelOwnedByACNS:   ac.Namespace,
		labelOwnedByACName: ac.Name,
		labelSpecHash:      d.SpecHash,
	}
	cur := &nlbv1alpha1.Listener{}
	if err := r.Client.Get(ctx, types.NamespacedName{Name: d.ListenerName}, cur); err != nil {
		if !apierrors.IsNotFound(err) {
			return false, err
		}
		port, perr := toPort(d.Port)
		if perr != nil {
			return false, fmt.Errorf("invalid listener port %q: %w", d.Port, perr)
		}
		lsn := &nlbv1alpha1.Listener{
			ObjectMeta: metav1.ObjectMeta{
				Name:   d.ListenerName,
				Labels: labels,
			},
			Spec: nlbv1alpha1.ListenerSpec{
				ResourceSpec: xpv1.ResourceSpec{
					ProviderConfigReference: &xpv1.Reference{Name: providerConfigName},
				},
				ForProvider: nlbv1alpha1.ListenerParameters{
					ListenerProtocol: &d.Protocol,
					ListenerPort:     &port,
					LoadBalancerID:   &d.LoadBalancerID,
					ServerGroupID:    &serverGroupID,
				},
			},
		}
		if err := r.Client.Create(ctx, lsn); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AliyunClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1beta2.AliyunCluster{}).
		Named("infrastructure-aliyuncluster").
		Complete(r)
}
