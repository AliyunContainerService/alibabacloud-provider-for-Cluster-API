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

package v1beta2

import (
	"errors"
	"fmt"
	"net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonapi "cluster-api-provider-aliyun/api/common"
	"cluster-api-provider-aliyun/api/cs/v1alpha1"
)

// log is for logging in this package.
var mcpLog = logf.Log.WithName("aliyunmanagedcontrolplane-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *AliyunManagedControlPlane) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-controlplane-cluster-x-k8s-io-v1beta2-aliyunmanagedcontrolplane,mutating=true,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=aliyunmanagedcontrolplanes,verbs=create;update,versions=v1beta2,name=maliyunmanagedcontrolplane.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &AliyunManagedControlPlane{}

var (
	defClusterDomain string = "cluster.local"
	defClusterSpec   string = "ack.standard"
	clusterSpecs            = map[string]byte{
		"ack.standard":  0,
		"ack.pro.small": 0,
	}
	defProxyMode string = "ipvs"
)

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *AliyunManagedControlPlane) Default() {
	mcpLog.V(5).Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
	if r.Spec.ClusterName == "" {
		mcpLog.Info("ClusterName empty, generate")
		name, err := generateClusterName(r.Name, r.Namespace)
		if err != nil {
			mcpLog.Error(err, "failed to create EKS cluster name")
			return
		}

		mcpLog.Info("defaulting cluster name", "cluster", name)
		r.Spec.ClusterName = name
	}
	if r.Spec.ClusterDomain == "" {
		mcpLog.Info("ClusterDomain empty, use default", "value", defClusterDomain)
		r.Spec.ClusterDomain = defClusterDomain
	}
	if r.Spec.ClusterSpec == "" {
		mcpLog.Info("ClusterSpec empty, use default", "value", defClusterSpec)
		r.Spec.ClusterSpec = defClusterSpec
	}
	if r.Spec.KubeProxy.ProxyMode == "" {
		mcpLog.Info("ProxyMode empty, use default", "value", defProxyMode)
		r.Spec.KubeProxy.ProxyMode = defProxyMode
	}

	// todo: 移除 disable 代码
	var found bool
	var True bool = true
	var ingressName string = "nginx-ingress-controller"
	for _, addon := range r.Spec.Addons {
		if *addon.Name == ingressName {
			found = true
			addon.Disabled = &True
		}
	}
	if !found {
		r.Spec.Addons = append(r.Spec.Addons, v1alpha1.AddonsParameters{
			Name:     &ingressName,
			Disabled: &True,
		})
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-controlplane-cluster-x-k8s-io-v1beta2-aliyunmanagedcontrolplane,mutating=false,failurePolicy=fail,sideEffects=None,groups=controlplane.cluster.x-k8s.io,resources=aliyunmanagedcontrolplanes,verbs=create;update,versions=v1beta2,name=valiyunmanagedcontrolplane.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &AliyunManagedControlPlane{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *AliyunManagedControlPlane) ValidateCreate() (admission.Warnings, error) {
	mcpLog.V(5).Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.

	var allErrs field.ErrorList

	allErrs = append(allErrs, r.validateClusterName()...)

	if r.Spec.Region == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec.region"), "Region is required"))
	}
	if _, ok := clusterSpecs[r.Spec.ClusterSpec]; !ok {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec.ClusterSpec"), r.Spec.ClusterSpec, "ClusterSpec must be one of [ack.standard, ack.pro.small]"),
		)
	}

	// if r.Spec.Network.ServiceCIDR == "" {
	// 	allErrs = append(allErrs, field.Required(field.NewPath("spec.network.serviceCIDR"), "ServiceCIDR is required"))
	// }
	// if r.Spec.Network.PodCIDR == "" {
	// 	allErrs = append(allErrs, field.Required(field.NewPath("spec.network.podCIDR"), "PodCIDR is required"))
	// }

	if r.Spec.KubeProxy.ProxyMode != "iptables" && r.Spec.KubeProxy.ProxyMode != "ipvs" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec.kubeProxy.proxyMode"), "ProxyMode must be one of [iptables, ipvs]"))
	}

	////////////////////////////////////////////////////////////////////////////
	// vpc 与 vswitch 的校验逻辑较为复杂
	//
	// 1). vpc和vswitch的id字段与其他字段均互斥，指定id时不能指定其他字段，反之亦然
	// 2). 使用已有vpc时，可使用已有/自动创建vswitch；自动创建vpc时，只能自动创建vswitch
	// 3). 控制面的vpc和vswitch均不可变更

	// if r.Spec.Network.VpcID == "" {
	// 	allErrs = append(allErrs, field.Required(field.NewPath("spec.network.vpcId"), "VpcID is required"))
	// }
	// 1. 不管 vpc 是否指定, 是否需要自建, 用户必须指定 vswitch , 因此先校验 vswitch 的数量.
	switchNums := len(r.Spec.Network.VSwitches)
	if switchNums < 1 || switchNums > 5 {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec.network.vSwitches"), "Please specify 1-5 VSwitches",
		))
	}
	// 2. vswitch id 与其他字段的互斥校验
	for i := range r.Spec.Network.VSwitches {
		vsw := &r.Spec.Network.VSwitches[i]
		if vsw.ID != "" {
			if vsw.Name != "" || vsw.CIDRBlock != "" {
				allErrs = append(allErrs, field.Invalid(
					field.NewPath("spec.network.vSwitches[]"), vsw,
					"Do not specify name/cidrBlock for a vpc with id field",
				))
			}
		} else {
			// 自建 vswitch 字段校验
			if vsw.Name == "" || vsw.CIDRBlock == "" {
				allErrs = append(allErrs, field.Invalid(
					field.NewPath("spec.network.vSwitches[]"), vsw,
					"Please specify name/cidrBlock for a vpc without id field",
				))
			} else {
				if _, _, err := net.ParseCIDR(vsw.CIDRBlock); err != nil {
					allErrs = append(allErrs, field.Invalid(
						field.NewPath("spec.network.vSwitches[].cidrBlock"), vsw.CIDRBlock,
						"Invalid cidrBlock",
					))
				}
				if vsw.Description == "" {
					vsw.Description = commonapi.GenerateVSwitchDescription(
						r.Kind, r.Spec.ClusterName,
					)
				}
			}
		}
	}

	// 3. 联合校验
	if r.Spec.Network.Vpc.ID == "" && r.Spec.Network.Vpc.Name == "" {
		// 未指定 vpc, 则 vswitch 只能指定已有 vswitch, 无法自建
		for _, vsw := range r.Spec.Network.VSwitches {
			if vsw.ID != "" {
				continue
			}
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec.network.vSwitches[]"), vsw,
				fmt.Sprintf("Please specify id for vswitch %s with empty vpc block", vsw.Name),
			))
		}
	} else if r.Spec.Network.Vpc.ID != "" && r.Spec.Network.Vpc.Name == "" {
		// 已有 vpc, 可以指定已有 vswitch, 也可以自建
	} else if r.Spec.Network.Vpc.ID == "" && r.Spec.Network.Vpc.Name != "" {
		// 自建 vpc, 则 vswitch 只能自建
		if _, _, err := net.ParseCIDR(r.Spec.Network.Vpc.CIDRBlock); err != nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec.network.vpc.cidrBlock"), r.Spec.Network.Vpc.CIDRBlock,
				"Invalid cidrBlock",
			))
		}
		for _, vsw := range r.Spec.Network.VSwitches {
			if vsw.ID == "" {
				continue
			}
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec.network.vSwitches[]"), vsw,
				fmt.Sprintf(
					"Do not specify id field for vswitch with non exist vpc %s",
					r.Spec.Network.Vpc.Name,
				),
			))
		}
		if r.Spec.Network.Vpc.Description == "" {
			r.Spec.Network.Vpc.Description = commonapi.GenerateVPCDescription(r.Spec.ClusterName)
		}
	} else {
		// 非法 vpc, id 与 name 不可同时指定
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec.network.vpc"), r.Spec.Network.Vpc,
			"Do not specify name/cidrBlock for a vpc with id field",
		))
	}
	////////////////////////////////////////////////////////////////////////////

	// TODO: Add ipv6 validation things in these validations.
	allErrs = append(allErrs, r.Spec.AdditionalTags.Validate()...)

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		r.GroupVersionKind().GroupKind(), r.Name, allErrs,
	)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *AliyunManagedControlPlane) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	mcpLog.V(5).Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	oldAliyunManagedControlplane, ok := old.(*AliyunManagedControlPlane)
	if !ok {
		return nil, apierrors.NewInvalid(GroupVersion.WithKind("AliyunManagedControlPlane").GroupKind(), r.Name, field.ErrorList{
			field.InternalError(nil, errors.New("failed to convert old AliyunManagedControlPlane to object")),
		})
	}
	var allErrs field.ErrorList

	// update 时不需要判空, 因为空值会被忽略

	allErrs = append(allErrs, r.validateClusterName()...)

	if r.Spec.Region != oldAliyunManagedControlplane.Spec.Region {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "region"), r.Spec.Region, "field is immutable"),
		)
	}
	if r.Spec.ClusterSpec != oldAliyunManagedControlplane.Spec.ClusterSpec {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "clusterSpec"), r.Spec.ClusterSpec, "field is immutable"),
		)
	}
	if r.Spec.ClusterDomain != oldAliyunManagedControlplane.Spec.ClusterDomain {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "clusterDomain"), r.Spec.ClusterDomain, "field is immutable"),
		)
	}
	if r.Spec.ResourceGroup != oldAliyunManagedControlplane.Spec.ResourceGroup {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "resourceGroup"), r.Spec.ResourceGroup, "field is immutable"),
		)
	}
	if r.Spec.TimeZone != oldAliyunManagedControlplane.Spec.TimeZone {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "timeZone"), r.Spec.TimeZone, "field is immutable"),
		)
	}
	if !r.Spec.Network.Vpc.Equal(oldAliyunManagedControlplane.Spec.Network.Vpc) {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "network", "vpc"), r.Spec.Network.Vpc, "field is immutable"),
		)
	}
	if !r.Spec.Network.VSwitches.Equal(oldAliyunManagedControlplane.Spec.Network.VSwitches) {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec", "network", "vSwitches"), r.Spec.Network.VSwitches, "field is immutable",
		))
	}

	// if r.Spec.Network.ServiceCIDR != oldAliyunManagedControlplane.Spec.Network.ServiceCIDR {
	// 	allErrs = append(allErrs,
	// 		field.Invalid(field.NewPath("spec", "network", "serviceCIDR"), r.Spec.Network.ServiceCIDR, "field is immutable"),
	// 	)
	// }
	// if r.Spec.Network.PodCIDR != oldAliyunManagedControlplane.Spec.Network.PodCIDR {
	// 	allErrs = append(allErrs,
	// 		field.Invalid(field.NewPath("spec", "network", "podCIDR"), r.Spec.Network.PodCIDR, "field is immutable"),
	// 	)
	// }
	if r.Spec.Network.NatGateway != oldAliyunManagedControlplane.Spec.Network.NatGateway {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "network", "natGateway"), r.Spec.Network.NatGateway, "field is immutable"),
		)
	}
	if r.Spec.Network.SecurityGroup.Create != oldAliyunManagedControlplane.Spec.Network.SecurityGroup.Create {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "network", "securityGroup", "create"), r.Spec.Network.SecurityGroup.Create, "field is immutable"),
		)
	}
	if r.Spec.Network.SecurityGroup.Enterprice != oldAliyunManagedControlplane.Spec.Network.SecurityGroup.Enterprice {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "network", "securityGroup", "enterprice"), r.Spec.Network.SecurityGroup.Enterprice, "field is immutable"),
		)
	}
	if r.Spec.Network.SecurityGroup.ID != oldAliyunManagedControlplane.Spec.Network.SecurityGroup.ID {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "network", "securityGroup", "id"), r.Spec.Network.SecurityGroup.ID, "field is immutable"),
		)
	}
	if r.Spec.CNI.Disable != oldAliyunManagedControlplane.Spec.CNI.Disable {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "cni", "disable"), r.Spec.CNI.Disable, "field is immutable"),
		)
	}
	if r.Spec.EntryptionConfig.ProviderKey != oldAliyunManagedControlplane.Spec.EntryptionConfig.ProviderKey {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "entryptionConfig", "providerKey"), r.Spec.EntryptionConfig.ProviderKey, "field is immutable"),
		)
	}
	if r.Spec.KubeProxy.ProxyMode != oldAliyunManagedControlplane.Spec.KubeProxy.ProxyMode {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "kubeProxy", "proxyMode"), r.Spec.KubeProxy.ProxyMode, "field is immutable"),
		)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		r.GroupVersionKind().GroupKind(), r.Name, allErrs,
	)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *AliyunManagedControlPlane) ValidateDelete() (admission.Warnings, error) {
	mcpLog.V(5).Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
