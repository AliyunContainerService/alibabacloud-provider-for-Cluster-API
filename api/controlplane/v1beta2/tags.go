/*
Copyright 2022 The Kubernetes Authors.

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
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// AdditionalTags defines a map of tags.
type AdditionalTags map[string]*string

// 1. key 为1-64个字符; value 为0-128个字符;
// 2. key 与 value 都不可以以"aliyun"、"acs:"、"https://"或"http://"开头
// 3. 最多设置20对 key-value;
// Keys and Values can only have alphabets, numbers, spaces and _ . : / = + - @ as characters.
// Tag's key cannot have prefix "aws:".
func (t AdditionalTags) Validate() []*field.Error {
	// Defines the maximum number of user tags which can be created for a specific resource
	const maxUserTagsAllowed = 20
	var errs field.ErrorList
	var userTagCount = len(t)
	re := regexp.MustCompile(`^[a-zA-Z0-9\s\_\.\:\=\+\-\@\/]*$`)

	for k, v := range t {
		if len(k) < 1 {
			errs = append(errs,
				field.Invalid(field.NewPath("spec", "additionalTags"), k, "key cannot be empty"),
			)
		}
		if len(k) > 64 {
			errs = append(errs,
				field.Invalid(field.NewPath("spec", "additionalTags"), k, "key cannot be longer than 128 characters"),
			)
		}
		if len(*v) > 128 {
			errs = append(errs,
				field.Invalid(field.NewPath("spec", "additionalTags"), v, "value cannot be longer than 256 characters"),
			)
		}
		if wrongUserTagNomenclature(k) {
			errs = append(errs,
				field.Invalid(field.NewPath("spec", "additionalTags"), k, "user created tag's key cannot have such prefix: 'aliyun'、'acs:'、'https://'或'http://'"),
			)
		}
		if wrongUserTagNomenclature(*v) {
			errs = append(errs,
				field.Invalid(field.NewPath("spec", "additionalTags"), k, "user created tag's key cannot have such prefix: 'aliyun'、'acs:'、'https://'或'http://'"),
			)
		}
		val := re.MatchString(k)
		if !val {
			errs = append(errs,
				field.Invalid(field.NewPath("spec", "additionalTags"), k, "key cannot have characters other than alphabets, numbers, spaces and _ . : / = + - @ ."),
			)
		}
		val = re.MatchString(*v)
		if !val {
			errs = append(errs,
				field.Invalid(field.NewPath("spec", "additionalTags"), v, "value cannot have characters other than alphabets, numbers, spaces and _ . : / = + - @ ."),
			)
		}
	}

	if userTagCount > maxUserTagsAllowed {
		errs = append(errs,
			field.Invalid(field.NewPath("spec", "additionalTags"), t, "user created tags cannot be more than 50"),
		)
	}

	return errs
}

// Checks whether the tag created is user tag or not.
func wrongUserTagNomenclature(k string) bool {
	return strings.HasPrefix(k, "aliyun") || strings.HasPrefix(k, "acs") || strings.HasPrefix(k, "https://") || strings.HasPrefix(k, "http://")
}
