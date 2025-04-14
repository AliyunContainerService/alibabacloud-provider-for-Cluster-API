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

package common

import "fmt"

// GenerateVSwitchDescription ...
//
// @param ownerType: 可以是
func GenerateVSwitchDescription(ownerType, name string) (desc string) {
	if ownerType == "AliyunManagedControlPlane" {
		desc = fmt.Sprintf("vswitch for ack cluster: %s", name)
	} else if ownerType == "AliyunManagedMachinePool" {
		desc = fmt.Sprintf("vswitch for ack nodepool: %s", name)
	}
	return
}

type VSwitches []VSwitch
type VSwitch struct {
	ID string `json:"id,omitempty"` // 如 ID 不为空, 表示为已有实例(阿里云上已存在)

	Name        string `json:"name,omitempty"`
	ResourceID  string `json:"resourceID,omitempty"` // 自建 vswitch 时, 在阿里云创建成功后的实例 id
	UID         string `json:"uid,omitempty"`        // 自建 vswitch 时, 本地 vswitch 资源的 uid
	ZoneID      string `json:"zoneId,omitempty"`
	CIDRBlock   string `json:"cidrBlock,omitempty"`
	Description string `json:"description,omitempty"`
}

func (vsw VSwitch) Equal(old VSwitch) bool {
	return vsw.ID == old.ID && vsw.Name == old.Name && vsw.CIDRBlock == old.CIDRBlock &&
		// 允许信息回填
		((vsw.ResourceID != "" && old.ResourceID == "") || vsw.ResourceID == old.ResourceID)
}

func (vsws VSwitches) Equal(old VSwitches) bool {
	l1, l2 := len(vsws), len(old)
	if l1 != l2 {
		return false
	}
	// 顺序也不可变更
	for i, j := 0, 0; i < l1 && j < l2; i, j = i+1, j+1 {
		if !vsws[i].Equal(old[j]) {
			return false
		}
	}
	return true
}

// GetIDList 获取合法 vswitch id 列表, 如果因为创建中导致 resourceId 未回填, 则返回 err.
func (vsws VSwitches) GetIDList() (ids []*string, err error) {
	for _, vsw := range vsws {
		if vsw.ID != "" {
			// 陷阱: vsw 在遍历时会发生变化, 如果直接 &vsw.ID, 当下一个 vsw 进入 case 2,
			// 会把已经存在 ids 列表中的成员 id 置空.
			// ids = append(ids, &vsw.ID)
			id := vsw.ID
			ids = append(ids, &id)
		} else if vsw.ResourceID != "" && vsw.UID != "" {
			// ids = append(ids, &vsw.ResourceID)
			id := vsw.ResourceID
			ids = append(ids, &id)
		} else {
			err = fmt.Errorf("incomplete vswitch object: %+v", vsw)
			break
		}
	}
	return
}

// Diff 比较两个
func (vsws VSwitches) Diff(another VSwitches) {
	return
}
