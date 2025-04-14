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

package v1beta2

import (
	"errors"
	"fmt"
	"net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	kubesets "k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	commonapi "cluster-api-provider-aliyun/api/common"
)

// log is for logging in this package.
var mmpLog = logf.Log.WithName("aliyunmanagedmachinepool-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *AliyunManagedMachinePool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1beta2-aliyunmanagedmachinepool,mutating=true,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=aliyunmanagedmachinepools,verbs=create;update,versions=v1beta2,name=maliyunmanagedmachinepool.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &AliyunManagedMachinePool{}

var (
	maxPoolNameLength         = 63
	zero              float64 = 0

	defSystemDiskSize             float64 = 120
	minSystemDiskSize             float64 = 40
	maxSystemDiskSize             float64 = 500
	defSystemDiskCategory         string  = "cloud_efficiency"
	defSystemDiskPerformanceLevel string  = "PL1"
	systemDiskCategories                  = sets.NewString("cloud_ssd", "cloud_efficiency", "cloud_essd")
	systemDiskPerformanceLevels           = sets.NewString("PL0", "PL1", "PL2", "PL3")

	defDataDiskSize             float64 = 40
	minDataDiskSize             float64 = 40
	maxDataDiskSize             float64 = 32768
	defDataDiskCategory         string  = "cloud_efficiency"
	defDataDiskPerformanceLevel string  = "PL1"
	dataDiskCategories                  = sets.NewString("cloud", "cloud_ssd", "cloud_efficiency", "cloud_essd")
	dataDiskPerformanceLevels           = sets.NewString("PL0", "PL1", "PL2", "PL3")
)

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *AliyunManagedMachinePool) Default() {
	mmpLog.V(5).Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.

	if r.Spec.ScalingGroup.SystemDiskCategory == "" {
		mmpLog.Info("SystemDiskCategory empty, use default", "value", defSystemDiskCategory)
		r.Spec.ScalingGroup.SystemDiskCategory = defSystemDiskCategory
	}
	if r.Spec.ScalingGroup.SystemDiskSize == nil || *r.Spec.ScalingGroup.SystemDiskSize == zero {
		mmpLog.Info("SystemDiskSize empty, use default", "value", defSystemDiskSize)
		r.Spec.ScalingGroup.SystemDiskSize = &defSystemDiskSize
	}
	if r.Spec.ScalingGroup.SystemDiskPerformanceLevel == "" {
		mmpLog.Info("SystemDiskPerformanceLevel empty, use default", "value", defSystemDiskPerformanceLevel)
		r.Spec.ScalingGroup.SystemDiskPerformanceLevel = defSystemDiskPerformanceLevel
	}

	// todo: 移除默认值配置(字段留空也可以创建成功)
	if r.Spec.ScalingGroup.ImageType == "" {
		defImageType := "AliyunLinux3"
		mmpLog.Info("ImageType empty, use default", "value", defImageType)
		r.Spec.ScalingGroup.ImageType = defImageType
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1beta2-aliyunmanagedmachinepool,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=aliyunmanagedmachinepools,verbs=create;update,versions=v1beta2,name=valiyunmanagedmachinepool.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &AliyunManagedMachinePool{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *AliyunManagedMachinePool) ValidateCreate() (admission.Warnings, error) {
	mmpLog.V(5).Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	var allErrs field.ErrorList

	// if int(*r.Spec.ScalingGroup.DesiredSize) == 0 {
	// 	allErrs = append(allErrs, field.Required(field.NewPath("spec.scalingGroup.desiredSize"), "desiredSize is required"))
	// }
	if r.Spec.AckNodePoolName == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec.ackNodePoolName"), "AckNodePoolName is required",
		))
	}
	allErrs = append(allErrs, r.validatePoolName()...)

	if len(r.Spec.ScalingGroup.VSwitches) < 1 || len(r.Spec.ScalingGroup.VSwitches) > 8 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec.scalingGroup.vSwitches[]"),
			r.Spec.ScalingGroup.VSwitches,
			"Please specify 1-8 VSwitches",
		))
	}
	for i := range r.Spec.ScalingGroup.VSwitches {
		vsw := &r.Spec.ScalingGroup.VSwitches[i]
		if vsw.ID != "" {
			if vsw.Name != "" || vsw.CIDRBlock != "" {
				allErrs = append(allErrs, field.Invalid(
					field.NewPath("spec.scalingGroup.vSwitches[]"), vsw,
					"Do not specify name/cidrBlock for a vpc with id field",
				))
			}
		} else {
			// 自建 vswitch 字段校验
			if vsw.Name == "" || vsw.CIDRBlock == "" {
				allErrs = append(allErrs, field.Invalid(
					field.NewPath("spec.scalingGroup.vSwitches[]"), vsw,
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
						r.Kind, r.Spec.AckNodePoolName,
					)
				}
			}
		}
	}
	if len(r.Spec.ScalingGroup.InstanceTypes) == 0 {
		allErrs = append(allErrs, field.Required(field.NewPath("spec.scalingGroup.instanceTypes"), "instanceTypes is required"))
	}

	if !systemDiskCategories.Has(r.Spec.ScalingGroup.SystemDiskCategory) {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec.scalingGroup.systemDiskCategory"),
			r.Spec.ScalingGroup.SystemDiskCategory,
			fmt.Sprintf("systemDiskCategory must be one of %s", systemDiskCategories.List()),
		))
	}
	if *r.Spec.ScalingGroup.SystemDiskSize < minSystemDiskSize || *r.Spec.ScalingGroup.SystemDiskSize > maxSystemDiskSize {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec.scalingGroup.systemDiskSize"),
			r.Spec.ScalingGroup.SystemDiskSize,
			fmt.Sprintf("systemDiskSize must be [%d, %d]", int(minSystemDiskSize), int(maxSystemDiskSize)),
		))
	}
	if !systemDiskPerformanceLevels.Has(r.Spec.ScalingGroup.SystemDiskPerformanceLevel) {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec.scalingGroup.systemDiskPerformanceLevel"),
			r.Spec.ScalingGroup.SystemDiskPerformanceLevel,
			fmt.Sprintf("systemDiskPerformanceLevel must be one of %s", systemDiskPerformanceLevels.List()),
		))
	}

	for _, d := range r.Spec.ScalingGroup.DataDisks {
		if d.Category != nil && !dataDiskCategories.Has(*d.Category) {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec.scalingGroup.dataDisks[].Category"),
				d.Category,
				fmt.Sprintf("dataDisks[].Category must be one of %s", dataDiskCategories.List()),
			))
		}
		if d.Size != nil && *d.Size < minDataDiskSize || *d.Size > maxDataDiskSize {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec.scalingGroup.dataDisks[].Size"),
				d.Size,
				fmt.Sprintf("dataDisks[].Size must be [%d, %d]", int(minDataDiskSize), int(maxDataDiskSize)),
			))
		}
		if d.PerformanceLevel != nil && !dataDiskPerformanceLevels.Has(*d.PerformanceLevel) {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec.scalingGroup.dataDisks[].PerformanceLevel"),
				d.PerformanceLevel,
				fmt.Sprintf("dataDisks[].PerformanceLevel must be one of %s", dataDiskPerformanceLevels.List()),
			))
		}
	}

	// keyName 与 password 必须且只能选择一个.
	if (r.Spec.ScalingGroup.KeyName == "" && r.Spec.ScalingGroup.Password == "") || (r.Spec.ScalingGroup.KeyName != "" && r.Spec.ScalingGroup.Password != "") {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec.scalingGroup"),
			map[string]string{
				"keyName":  r.Spec.ScalingGroup.KeyName,
				"password": r.Spec.ScalingGroup.Password,
			},
			"You have to specify one of [password, keyName] fields",
		))
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		r.GroupVersionKind().GroupKind(),
		r.Name,
		allErrs,
	)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *AliyunManagedMachinePool) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	mmpLog.V(5).Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	oldAliyunManagedMachinePool, ok := old.(*AliyunManagedMachinePool)
	if !ok {
		return nil, apierrors.NewInvalid(GroupVersion.WithKind("AliyunManagedMachinePool").GroupKind(), r.Name, field.ErrorList{
			field.InternalError(nil, errors.New("failed to convert old AliyunManagedMachinePool to object")),
		})
	}
	var allErrs field.ErrorList

	// update 时不需要判空, 因为空值会被忽略

	// 数组判断相等(不考虑顺序)
	sgSetOld, sgSetNew := kubesets.NewString(), kubesets.NewString()
	for _, id := range oldAliyunManagedMachinePool.Spec.ScalingGroup.SecurityGroupIDs {
		sgSetOld.Insert(*id)
	}
	for _, id := range r.Spec.ScalingGroup.SecurityGroupIDs {
		sgSetNew.Insert(*id)
	}
	if sgSetOld.Len() != 0 && !sgSetOld.Equal(sgSetNew) {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "scalingGroup", "securityGroupIds"), r.Spec.ScalingGroup.SecurityGroupIDs, "field is immutable"),
		)
	}

	if r.Spec.ScalingGroup.ImageType != oldAliyunManagedMachinePool.Spec.ScalingGroup.ImageType {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "scalingGroup", "imageType"), r.Spec.ScalingGroup.ImageType, "field is immutable"),
		)
	}

	if !systemDiskCategories.Has(r.Spec.ScalingGroup.SystemDiskCategory) {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec.scalingGroup.systemDiskCategory"),
			r.Spec.ScalingGroup.SystemDiskCategory,
			fmt.Sprintf("systemDiskCategory must be one of %s", systemDiskCategories.List()),
		))
	}
	if *r.Spec.ScalingGroup.SystemDiskSize < minSystemDiskSize || *r.Spec.ScalingGroup.SystemDiskSize > maxSystemDiskSize {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec.scalingGroup.systemDiskSize"),
			r.Spec.ScalingGroup.SystemDiskSize,
			fmt.Sprintf("systemDiskSize must be [%d~%d] in GB", int(minSystemDiskSize), int(maxSystemDiskSize)),
		))
	}
	if !systemDiskPerformanceLevels.Has(r.Spec.ScalingGroup.SystemDiskPerformanceLevel) {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec.scalingGroup.systemDiskPerformanceLevel"),
			r.Spec.ScalingGroup.SystemDiskPerformanceLevel,
			fmt.Sprintf("systemDiskPerformanceLevel must be one of %s", systemDiskPerformanceLevels.List()),
		))
	}

	for _, d := range r.Spec.ScalingGroup.DataDisks {
		if d.Category != nil && !dataDiskCategories.Has(*d.Category) {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec.scalingGroup.dataDisks[].Category"),
				d.Category,
				fmt.Sprintf("dataDisks[].Category must be one of %s", dataDiskCategories.List()),
			))
		}
		if d.Size != nil && *d.Size < minDataDiskSize || *d.Size > maxDataDiskSize {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec.scalingGroup.dataDisks[].Size"),
				d.Size,
				fmt.Sprintf("dataDisks[].Size must be [%d, %d]", int(minDataDiskSize), int(maxDataDiskSize)),
			))
		}
		if d.PerformanceLevel != nil && !dataDiskPerformanceLevels.Has(*d.PerformanceLevel) {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("spec.scalingGroup.dataDisks[].PerformanceLevel"),
				d.PerformanceLevel,
				fmt.Sprintf("dataDisks[].PerformanceLevel must be one of %s", dataDiskPerformanceLevels.List()),
			))
		}
	}

	for i := range r.Spec.ScalingGroup.VSwitches {
		vsw := &r.Spec.ScalingGroup.VSwitches[i]
		if vsw.ID != "" {
			if vsw.Name != "" || vsw.CIDRBlock != "" {
				allErrs = append(allErrs, field.Invalid(
					field.NewPath("spec.scalingGroup.vSwitches[]"), vsw,
					"Do not specify name/cidrBlock for a vpc with id field",
				))
			}
		} else {
			// 自建 vswitch 字段校验
			if vsw.Name == "" || vsw.CIDRBlock == "" || vsw.ZoneID == "" {
				allErrs = append(allErrs, field.Invalid(
					field.NewPath("spec.scalingGroup.vSwitches[]"), vsw,
					"Please specify name/cidrBlock for a vpc without id field",
				))
			} else if _, _, err := net.ParseCIDR(vsw.CIDRBlock); err != nil {
				allErrs = append(allErrs, field.Invalid(
					field.NewPath("spec.network.vSwitches[].cidrBlock"), vsw.CIDRBlock,
					"Invalid cidrBlock",
				))
			} else {
				// 字段完整, 则需要与原数组中同名的 vswitch 成员比对, 其余字段不可变更.
				for _, oldVsw := range oldAliyunManagedMachinePool.Spec.ScalingGroup.VSwitches {
					if oldVsw.Name != vsw.Name {
						continue
					}
					if oldVsw.CIDRBlock != vsw.CIDRBlock || oldVsw.ZoneID != vsw.ZoneID {
						allErrs = append(allErrs, field.Invalid(
							field.NewPath("spec.network.vSwitches[]"), vsw,
							"cidrBlock, zoneId is immutable",
						))
					}
				}
				if vsw.Description == "" {
					vsw.Description = commonapi.GenerateVSwitchDescription(
						r.Kind, r.Spec.AckNodePoolName,
					)
				}
			}
		}
	}

	// keyName 与 password 必须且只能选择一个.
	// todo: 实践发现, 原本为 keyName 认证变更为 password 后, 在apply会报错(keyName无法被真正的设置为 nil)
	if (r.Spec.ScalingGroup.KeyName == "" && r.Spec.ScalingGroup.Password == "") || (r.Spec.ScalingGroup.KeyName != "" && r.Spec.ScalingGroup.Password != "") {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec.scalingGroup"),
			map[string]string{
				"keyName":  r.Spec.ScalingGroup.KeyName,
				"password": r.Spec.ScalingGroup.Password,
			},
			"You have to specify one of [password, keyName] fields",
		))
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(
		r.GroupVersionKind().GroupKind(),
		r.Name,
		allErrs,
	)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *AliyunManagedMachinePool) ValidateDelete() (admission.Warnings, error) {
	mmpLog.V(5).Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
