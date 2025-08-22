package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type AliyunMachineTemplateSpec struct {
	// Template 为创建 AliyunMachine（单机规格）的模板。
	Template AliyunMachineTemplateResource `json:"template"`
}

// AliyunMachineTemplateResource 与 CAPI 常见 *MachineTemplate 结构一致：
type AliyunMachineTemplateResource struct {
	// +optional
	Metadata metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec     AliyunMachineSpec `json:"spec"`
}

// SystemDiskSpec 映射 Upjet Instance 的 system_disk* 相关字段。
type SystemDiskSpec struct {
	// 类别：cloud_efficiency / cloud_ssd / cloud_essd / cloud_auto ...
	// 对应 InstanceParameters.SystemDiskCategory
	// +optional
	Category *string `json:"category,omitempty"`

	// ESSD 性能等级：PL0/PL1/PL2/PL3
	// +optional
	PerformanceLevel *string `json:"performanceLevel,omitempty"`

	Size *int32 `json:"size,omitempty"`

	// 是否加密/算法/KMS
	// +optional
	Encrypted         *bool   `json:"encrypted,omitempty"`
	EncryptAlgorithm  *string `json:"encryptAlgorithm,omitempty"` // aes-256 / sm4-128
	KMSKeyID          *string `json:"kmsKeyId,omitempty"`
	AutoSnapshotPolID *string `json:"autoSnapshotPolicyId,omitempty"`
	// +optional
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// DataDiskSpec（可选）映射 InstanceParameters.data_disks[*]
type DataDiskSpec struct {
	// +optional
	Category *string `json:"category,omitempty"`
	// +optional
	PerformanceLevel *string `json:"performanceLevel,omitempty"`
	// +optional
	Size *int32 `json:"size,omitempty"`
	// +optional
	SnapshotID *string `json:"snapshotId,omitempty"`
	// +optional
	DeleteWithInstance *bool `json:"deleteWithInstance,omitempty"`
	// +optional
	KMSKeyID *string `json:"kmsKeyId,omitempty"`
	// +optional
	Encrypted *bool `json:"encrypted,omitempty"`
	// +optional
	Name *string `json:"name,omitempty"`
	// +optional
	Description *string `json:"description,omitempty"`
	// +optional
	Device *string `json:"device,omitempty"`
}

// AliyunMachineSpec —— 你的控制器会把这些字段映射到 Upjet ecs.Instance.spec.forProvider
// 以及把 Bootstrap 的 userData Secret 透传为 Upjet 的 user_data。
type AliyunMachineSpec struct {
	ProviderID string `json:"providerID,omitempty"`

	// 地域，如 cn-hangzhou
	RegionID string `json:"regionId"`

	// 交换机（子网）ID（必填，除非你支持经典网络）
	VSwitchID string `json:"vSwitchId"`

	// 安全组 ID 列表
	// 对应 InstanceParameters.SecurityGroups
	// +optional
	SecurityGroupIDs []string `json:"securityGroupIds,omitempty"`

	// 可选：实例私网 IP（不指定则自动分配）
	// +optional
	PrivateIP *string `json:"privateIp,omitempty"`

	// 实例规格
	InstanceType string `json:"instanceType"`

	// 镜像 ID（如公共镜像/自定义镜像）
	// +optional
	ImageID *string `json:"imageId,omitempty"`

	// 登录密钥对名称（设置后通常禁用密码登录）
	// +optional
	KeyName *string `json:"keyName,omitempty"`

	// ---------------- 磁盘 ----------------
	// 系统盘配置
	// +optional
	SystemDisk *SystemDiskSpec `json:"systemDisk,omitempty"`

	// 数据盘配置
	// +optional
	DataDisks []DataDiskSpec `json:"dataDisks,omitempty"`

	// 公网出带宽，Mbps。为 0 表示仅私网
	// 对应 InstanceParameters.InternetMaxBandwidthOut
	// +optional
	InternetMaxBandwidthOut *int32 `json:"internetMaxBandwidthOut,omitempty"`

	// 公网入带宽
	// +optional
	InternetMaxBandwidthIn *int32 `json:"internetMaxBandwidthIn,omitempty"`

	// 计费类型：PostPaid / PrePaid（不写默认 PostPaid）
	// +optional
	InstanceChargeType *string `json:"instanceChargeType,omitempty"`

	// 计费类型：PayByBandwidth / PayByTraffic（不写默认 PayByTraffic）
	// +optional
	InternetChargeType *string `json:"internetChargeType,omitempty"`

	// 资源标签：会同步到 Upjet Instance.tags
	// +optional
	Tags map[string]string `json:"tags,omitempty"`

	// 写入到磁盘设备的标签（Upjet Instance.volume_tags）
	// +optional
	VolumeTags map[string]string `json:"volumeTags,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=aliyunmachinetemplates,scope=Namespaced,shortName=alimpctl,categories=cluster-api
// +kubebuilder:printcolumn:name="INSTANCE",type=string,JSONPath=".spec.template.spec.instanceType",priority=0
// +kubebuilder:printcolumn:name="VSWITCH",type=string,JSONPath=".spec.template.spec.vSwitchId",priority=1
// +kubebuilder:printcolumn:name="REGION",type=string,JSONPath=".spec.template.spec.regionId",priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
type AliyunMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AliyunMachineTemplateSpec `json:"spec,omitempty"`
	Status metav1.ConditionStatus    `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type AliyunMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AliyunMachineTemplate `json:"items"`
}

var (
	AliyunMachineTemplateKind             = "AliyunMachineTemplate"
	AliyunMachineTemplateGroupKind        = schema.GroupKind{Group: "infrastructure.cluster.x-k8s.io", Kind: AliyunMachineTemplateKind}.String()
	AliyunMachineTemplateKindAPIVersion   = AliyunMachineTemplateKind + "." + "v1beta1"
	AliyunMachineTemplateGroupVersionKind = schema.GroupVersion{Group: "infrastructure.cluster.x-k8s.io", Version: "v1beta1"}.WithKind(AliyunMachineTemplateKind)
)

func init() {
	SchemeBuilder.Register(&AliyunMachineTemplate{}, &AliyunMachineTemplateList{})
}
