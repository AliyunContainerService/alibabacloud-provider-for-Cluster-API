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

package v1beta2

import (
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// GetConditions returns the observations of the operational state of the AliyunManagedMachinePool resource.
func (r *AliyunManagedMachinePool) GetConditions() clusterv1.Conditions {
	return r.Status.Conditions
}

// SetConditions sets the underlying service state of the AliyunManagedMachinePool to the predescribed clusterv1.Conditions.
func (r *AliyunManagedMachinePool) SetConditions(conditions clusterv1.Conditions) {
	r.Status.Conditions = conditions
}

func (r *AliyunManagedMachinePool) validatePoolName() field.ErrorList {
	var allErrs field.ErrorList

	// 长度限制
	if len(r.Spec.AckNodePoolName) > maxPoolNameLength {
		allErrs = append(allErrs, field.TooLongMaxLength(field.NewPath("spec.clusterName"), r.Spec.AckNodePoolName, maxPoolNameLength))
	}
	// 不可以 - _ 开头
	if strings.HasPrefix(r.Spec.AckNodePoolName, "-") || strings.HasPrefix(r.Spec.AckNodePoolName, "_") {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec.clusterName"), r.Spec.AckNodePoolName, "cluster name cannot start with - or _"))
	}
	// 除了 - _ 外, 不可包含特殊符号(但理论上可以包含中文)
	re := regexp.MustCompile(`^[a-zA-Z0-9\_\-]*$`)
	val := re.MatchString(r.Spec.AckNodePoolName)
	if !val {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec.clusterName"), r.Spec.AckNodePoolName, "cluster name cannot have characters other than alphabets, numbers and - _"))
	}
	return allErrs
}
