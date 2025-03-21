package common

import "fmt"

func GenerateVPCDescription(clusterName string) string {
	return fmt.Sprintf("vpc for ack cluster: %s", clusterName)
}

type VPC struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	ResourceID  string `json:"resourceID,omitempty"` // 自建 vpc 时, 在阿里云创建成功后的实例 id
	UID         string `json:"uid,omitempty"`        // 自建 vpc 时, 本地 vpc 资源的 uid
	CIDRBlock   string `json:"cidrBlock,omitempty"`
	Description string `json:"description,omitempty"`
}

func (vpc VPC) Equal(old VPC) bool {
	return vpc.ID == old.ID && vpc.Name == old.Name && vpc.CIDRBlock == old.CIDRBlock &&
		// 允许信息回填
		((vpc.ResourceID != "" && old.ResourceID == "") || vpc.ResourceID == old.ResourceID)
}
