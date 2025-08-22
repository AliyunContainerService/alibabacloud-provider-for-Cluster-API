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
	ecsv1alpha1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/ecs/v1alpha1"
	essv1alpha1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/ess/v1alpha1"
	nlbv1alpha1 "github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/nlb/v1alpha1"
	ess20220222 "github.com/alibabacloud-go/ess-20220222/v2/client"
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"net"
	"reflect"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sort"
	"strings"
)

// getOwnerMachinePool 查询目标资源 obj (aliyunPool)的 MachinePool 属主并返回.
//
//	@param obj: aliyunPool.ObjectMeta 成员字段
//
// getOwnerMachinePool returns the MachinePool object owning the current resource.
//func getOwnerMachinePool(ctx context.Context, c client.Client, obj metav1.ObjectMeta) (*expclusterv1.MachinePool, error) {
//	for _, ref := range obj.OwnerReferences {
//		if ref.Kind != "MachinePool" {
//			continue
//		}
//		gv, err := schema.ParseGroupVersion(ref.APIVersion)
//		if err != nil {
//			return nil, errors.WithStack(err)
//		}
//		if gv.Group == expclusterv1.GroupVersion.Group {
//			return getMachinePoolByName(ctx, c, obj.Namespace, ref.Name)
//		}
//	}
//	return nil, nil
//}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
func f(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
func ptrIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func f64PtrToI32PtrUnsafe(f *float64) *int32 {
	if f == nil {
		return nil
	}
	v := int32(*f) // 直接截断
	return &v
}

func stringSlicePtrEqual(a, b []*string) bool {
	if len(a) != len(b) {
		return false
	}
	// 无序比较
	set := map[string]int{}
	for _, p := range a {
		set[str(p)]++
	}
	for _, p := range b {
		set[str(p)]--
	}
	for _, v := range set {
		if v != 0 {
			return false
		}
	}
	return true
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func f64Ptr(v int) *float64 { f := float64(v); return &f }

func toPtrSlice(ss []string) []*string {
	out := make([]*string, 0, len(ss))
	for i := range ss {
		s := ss[i]
		out = append(out, &s)
	}
	return out
}

func toFloat64Ptr(i *int32) *float64 {
	if i == nil {
		return nil
	}
	v := float64(*i)
	return &v
}

func toPtrMap(m map[string]string) map[string]*string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]*string, len(m))
	for k, v := range m {
		val := v
		out[k] = &val
	}
	return out
}

func setEqPtr(a, b []*string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, p := range a {
		counts[str(p)]++
	}
	for _, p := range b {
		v := str(p)
		if counts[v] == 0 {
			return false
		}
		counts[v]--
	}
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}
	return true
}

func mapEq(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || va != vb {
			return false
		}
	}
	return true
}

func mapPtrEq(a, b map[string]*string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || str(va) != str(vb) {
			return false
		}
	}
	return true
}

func floatPtrEq(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	const eps = 1e-9
	diff := *a - *b
	if diff < 0 {
		diff = -diff
	}
	return diff <= eps
}

func providerRefEqual(a, b *xpv1.Reference) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Name == b.Name
}

// 只比较我们关心的字段，避免被 Upjet 的内部回写干扰幂等性
func equalScalingGroupParams(a, b essv1alpha1.ScalingGroupParameters) bool {
	if str(a.ScalingGroupName) != str(b.ScalingGroupName) {
		return false
	}
	if f(a.MinSize) != f(b.MinSize) {
		return false
	}
	if f(a.MaxSize) != f(b.MaxSize) {
		return false
	}
	if f(a.DesiredCapacity) != f(b.DesiredCapacity) {
		return false
	}
	if !stringSlicePtrEqual(a.VswitchIds, b.VswitchIds) {
		return false
	}
	if !stringSlicePtrEqual(a.LoadbalancerIds, b.LoadbalancerIds) {
		return false
	}
	return true
}

func equalScalingConfigurationParamsSliceAware(a, b essv1alpha1.ScalingConfigurationParameters) bool {
	if str(a.ScalingGroupID) != str(b.ScalingGroupID) {
		return false
	}
	if str(a.ImageID) != str(b.ImageID) {
		return false
	}
	if !stringSlicePtrEqual(a.InstanceTypes, b.InstanceTypes) {
		return false
	}
	if !stringSlicePtrEqual(a.SecurityGroupIds, b.SecurityGroupIds) {
		return false
	}
	if str(a.UserData) != str(b.UserData) {
		return false
	}
	if str(a.InternetChargeType) != str(b.InternetChargeType) {
		return false
	}
	if f(a.InternetMaxBandwidthIn) != f(b.InternetMaxBandwidthIn) {
		return false
	}
	if f(a.InternetMaxBandwidthOut) != f(b.InternetMaxBandwidthOut) {
		return false
	}
	return true
}

func calcReplicas(instances []*ess20220222.DescribeScalingInstancesResponseBodyScalingInstances) (int32, int32, error) {
	replicas := int32(len(instances))
	ready := int32(0)
	for _, inst := range instances {
		if inst.HealthStatus == nil || *inst.HealthStatus != "Healthy" {
			continue
		}
		if inst.InstanceId == nil {
			continue
		}
		ready++
	}
	return replicas, ready, nil
}

func ensureFinalizer(obj meta.Object, fin string) bool {
	finSet := obj.GetFinalizers()
	for _, f := range finSet {
		if f == fin {
			return false
		}
	}
	obj.SetFinalizers(append(finSet, fin))
	return true
}

func removeFinalizer(obj meta.Object, fin string) {
	var out []string
	for _, f := range obj.GetFinalizers() {
		if f != fin {
			out = append(out, f)
		}
	}
	obj.SetFinalizers(out)
}

// 只比较“可就地更新”的软字段，避免触发危险的 in-place 变更
func equalInstanceForProviderSoft(a, b ecsv1alpha1.InstanceParameters) bool {
	return mapPtrEq(a.Tags, b.Tags) &&
		mapPtrEq(a.VolumeTags, b.VolumeTags) &&
		setEqPtr(a.SecurityGroups, b.SecurityGroups) &&
		floatPtrEq(a.InternetMaxBandwidthOut, b.InternetMaxBandwidthOut) &&
		floatPtrEq(a.InternetMaxBandwidthIn, b.InternetMaxBandwidthIn)
}

func zvKey(z nlbv1alpha1.ZoneMappingsParameters) string {
	var zid, vsw string
	if z.ZoneID != nil {
		zid = strings.TrimSpace(*z.ZoneID)
	}
	if z.VswitchID != nil {
		vsw = strings.TrimSpace(*z.VswitchID)
	}
	return zid + "|" + vsw
}

func canonKeys(in []nlbv1alpha1.ZoneMappingsParameters) []string {
	keys := make([]string, 0, len(in))
	for _, z := range in {
		keys = append(keys, zvKey(z))
	}
	sort.Strings(keys) // 忽略顺序
	return keys
}

func zoneMappingsEqualByZoneAndVSwitch(a, b []nlbv1alpha1.ZoneMappingsParameters) bool {
	ak := canonKeys(a)
	bk := canonKeys(b)
	if len(ak) != len(bk) {
		return false
	}
	for i := range ak {
		if ak[i] != bk[i] {
			return false
		}
	}
	return true
}

func equalNlbForProviderSoft(a, b nlbv1alpha1.LoadBalancerParameters) bool {
	return zoneMappingsEqualByZoneAndVSwitch(a.ZoneMappings, b.ZoneMappings)
}

// 不可以更换的的参数
func hasHardImmutableDiff(a, b ecsv1alpha1.InstanceParameters) bool {
	aa, bb := a, b
	aa.Tags, bb.Tags = nil, nil
	aa.VolumeTags, bb.VolumeTags = nil, nil
	aa.SecurityGroups, bb.SecurityGroups = nil, nil
	aa.InternetMaxBandwidthOut, bb.InternetMaxBandwidthOut = nil, nil
	aa.InternetMaxBandwidthIn, bb.InternetMaxBandwidthIn = nil, nil

	return !reflect.DeepEqual(aa, bb)
}

func mergeSoftFields(cur, want *ecsv1alpha1.InstanceParameters) (changed bool) {
	if len(want.Tags) > 0 {
		if cur.Tags == nil {
			cur.Tags = make(map[string]*string, len(want.Tags))
		}
		for k, v := range want.Tags {
			if cv, ok := cur.Tags[k]; !ok || cv != v {
				cur.Tags[k] = v
				changed = true
			}
		}
	}

	// VolumeTags: upsert（只新增/更新，不删除）
	if len(want.VolumeTags) > 0 {
		if cur.VolumeTags == nil {
			cur.VolumeTags = make(map[string]*string, len(want.VolumeTags))
		}
		for k, v := range want.VolumeTags {
			if cv, ok := cur.VolumeTags[k]; !ok || cv != v {
				cur.VolumeTags[k] = v
				changed = true
			}
		}
	}

	// SecurityGroups: 只“补齐”缺少的（不移除已有的）
	if len(want.SecurityGroups) > 0 {
		exist := make(map[string]struct{}, len(cur.SecurityGroups))
		for _, p := range cur.SecurityGroups {
			exist[str(p)] = struct{}{}
		}
		for _, p := range want.SecurityGroups {
			val := str(p)
			if _, ok := exist[val]; !ok {
				// 注意：不能把 &val 直接 append（会引用同一地址）
				s := val
				cur.SecurityGroups = append(cur.SecurityGroups, &s)
				exist[val] = struct{}{}
				changed = true
			}
		}
	}

	// 带宽：对齐为 want 的值（如果不想降低，只在 want>cur 时赋值）
	if want.InternetMaxBandwidthOut != nil {
		if cur.InternetMaxBandwidthOut == nil || !floatPtrEq(cur.InternetMaxBandwidthOut, want.InternetMaxBandwidthOut) {
			cur.InternetMaxBandwidthOut = want.InternetMaxBandwidthOut
			changed = true
		}
	}
	if want.InternetMaxBandwidthIn != nil {
		if cur.InternetMaxBandwidthIn == nil || !floatPtrEq(cur.InternetMaxBandwidthIn, want.InternetMaxBandwidthIn) {
			cur.InternetMaxBandwidthIn = want.InternetMaxBandwidthIn
			changed = true
		}
	}

	return changed
}

func collectAddresses(inst *ecsv1alpha1.Instance) []clusterv1.MachineAddress {
	var out []clusterv1.MachineAddress
	add := func(t clusterv1.MachineAddressType, s string) {
		if s == "" {
			return
		}
		if t == clusterv1.MachineInternalIP || t == clusterv1.MachineExternalIP {
			if ip := net.ParseIP(s); ip == nil {
				return
			}
		}
		out = append(out, clusterv1.MachineAddress{Type: t, Address: s})
	}

	at := inst.Status.AtProvider
	if at.PrimaryIPAddress != nil {
		add(clusterv1.MachineInternalIP, *at.PrimaryIPAddress)
	}
	if at.PrivateIP != nil {
		add(clusterv1.MachineInternalIP, *at.PrivateIP)
	}
	if at.PublicIP != nil {
		add(clusterv1.MachineExternalIP, *at.PublicIP)
	}

	return out
}

func toPort(s string) (float64, error) {
	var p int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &p)
	return float64(p), err
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func ensureUpjetProviderConfigNoStore(
	ctx context.Context,
	c client.Client,
	group string,
	version string,
	name string,
	region string,
	secretNS, secretName, secretKey string,
) error {
	apiVersion := group + "/" + version

	u := &unstructured.Unstructured{}
	u.SetAPIVersion(apiVersion)
	u.SetKind("ProviderConfig")

	if err := c.Get(ctx, client.ObjectKey{Name: name}, u); err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       "ProviderConfig",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"region": region,
				"credentials": map[string]interface{}{
					"source": "Secret",
					"secretRef": map[string]interface{}{
						"namespace": secretNS,
						"name":      secretName,
						"key":       secretKey, // 例如 "credentials"
					},
				},
			},
		},
	}

	return c.Create(ctx, obj)
}
