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
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// GetConditions returns the observations of the operational state of the AliyunManagedControlPlane resource.
func (r *AliyunManagedControlPlane) GetConditions() clusterv1.Conditions {
	return r.Status.Conditions
}

// SetConditions sets the underlying service state of the AliyunManagedControlPlane to the predescribed clusterv1.Conditions.
func (r *AliyunManagedControlPlane) SetConditions(conditions clusterv1.Conditions) {
	r.Status.Conditions = conditions
}

func (r *AliyunManagedControlPlane) validateClusterName() field.ErrorList {
	var allErrs field.ErrorList

	// 长度限制
	if len(r.Spec.ClusterName) > maxClusterNameLength {
		allErrs = append(allErrs, field.TooLongMaxLength(field.NewPath("spec.clusterName"), r.Spec.ClusterName, maxClusterNameLength))
	}
	// 不可以 - _ 开头
	if strings.HasPrefix(r.Spec.ClusterName, "-") || strings.HasPrefix(r.Spec.ClusterName, "_") {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec.clusterName"), r.Spec.ClusterName, "cluster name cannot start with - or _"))
	}
	// 除了 - _ 外, 不可包含特殊符号(但理论上可以包含中文)
	re := regexp.MustCompile(`^[a-zA-Z0-9\_\-]*$`)
	val := re.MatchString(r.Spec.ClusterName)
	if !val {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec.clusterName"), r.Spec.ClusterName, "cluster name cannot have characters other than alphabets, numbers and - _"))
	}
	return allErrs
}

const (
	resourcePrefix             = "capa_"
	maxClusterNameLength       = 63
	generatedClusterNameLength = 32
)

// generateClusterName generates a name of an EKS resources.
func generateClusterName(resourceName, namespace string) (string, error) {
	escapedName := strings.ReplaceAll(resourceName, ".", "_")
	name := fmt.Sprintf("%s_%s", namespace, escapedName)

	// 如果 namespce/name 不超过限制长度, 则可以直接返回
	if len(name) < maxClusterNameLength {
		return name, nil
	}

	// 如果超过了, 则需要生成摘要信息, 默认生成32位.
	hashLength := generatedClusterNameLength - len(resourcePrefix)
	hashedName, err := Base36TruncatedHash(name, hashLength)
	if err != nil {
		return "", errors.Wrap(err, "creating hash from name")
	}

	return fmt.Sprintf("%s%s", resourcePrefix, hashedName), nil
}

const base36set = "0123456789abcdefghijklmnopqrstuvwxyz"

// Base36TruncatedHash 对目标 str 生成指定长度 length 的摘要字符串.
//
// Base36TruncatedHash returns a consistent hash using blake2b
// and truncating the byte values to alphanumeric only
// of a fixed length specified by the consumer.
func Base36TruncatedHash(str string, length int) (string, error) {
	hasher, err := blake2b.New(length, nil)
	if err != nil {
		return "", errors.Wrap(err, "unable to create hash function")
	}

	if _, err := hasher.Write([]byte(str)); err != nil {
		return "", errors.Wrap(err, "unable to write hash")
	}
	return base36Truncate(hasher.Sum(nil)), nil
}

// base36Truncate returns a string that is base36 compliant
// It is not an encoding since it returns a same-length string
// for any byte value.
func base36Truncate(bytes []byte) string {
	var chars string
	for _, bite := range bytes {
		idx := int(bite) % 36
		chars += string(base36set[idx])
	}

	return chars
}
