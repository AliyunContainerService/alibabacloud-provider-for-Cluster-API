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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

const (
	AliyunMachinePoolFinalizer = "aliyunmachinepool.infrastructure.cluster.x-k8s.io"
)

// AliyunMachinePoolSpec defines the desired state of AliyunMachinePool
type AliyunMachinePoolSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	ProviderID string `json:"providerID,omitempty"`

	// 伸缩配置：等同于 ScalingConfiguration
	// +kubebuilder:validation:Required
	ScalingConfiguration AliyunScalingConfigurationSpec `json:"scalingConfiguration"`

	// 伸缩组：等同于 ScalingGroup
	// +kubebuilder:validation:Required
	ScalingGroup AliyunScalingGroupSpec `json:"scalingGroup"`
}

// AliyunMachinePoolStatus defines the observed state of AliyunMachinePool.
type AliyunMachinePoolStatus struct {
	// ScalingGroupID is the ID of the underlying ESS ScalingGroup.
	ScalingGroupID string `json:"scalingGroupID,omitempty"`

	// ScalingConfigurationID is the ID of the ESS ScalingConfiguration used by the ScalingGroup.
	ScalingConfigurationID string `json:"scalingConfigurationID,omitempty"`

	// Replicas is the total number of instances in the ScalingGroup.
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of replicas that are ready and have a corresponding Kubernetes NodeReady condition.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Conditions defines current service state of the AliyunMachinePool.
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AliyunMachinePool is the Schema for the aliyunmachinepools API
type AliyunMachinePool struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of AliyunMachinePool
	// +required
	Spec AliyunMachinePoolSpec `json:"spec"`

	// status defines the observed state of AliyunMachinePool
	// +optional
	Status AliyunMachinePoolStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// AliyunMachinePoolList contains a list of AliyunMachinePool
type AliyunMachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AliyunMachinePool `json:"items"`
}

// 伸缩配置字段
type AliyunScalingConfigurationSpec struct {
	// +kubebuilder:validation:Required
	ScalingGroupID string `json:"scalingGroupId,omitempty"`
	// +kubebuilder:validation:Required
	ImageID string `json:"imageId"`
	// +kubebuilder:validation:MinItems=1
	InstanceTypes []string `json:"instanceTypes"`
	// +kubebuilder:validation:MinItems=1
	SecurityGroupIDs []string `json:"securityGroupIds"`
	// +optional
	UserData string `json:"userData,omitempty"`
	// Network billing type, Values: PayByBandwidth or PayByTraffic. Default to PayByBandwidth.
	// +kubebuilder:validation:Optional
	InternetChargeType *string `json:"internetChargeType,omitempty"`
	// Maximum incoming bandwidth from the public network, measured in Mbps (Mega bit per second). The value range is [1,200].
	// +kubebuilder:validation:Optional
	InternetMaxBandwidthIn *float64 `json:"internetMaxBandwidthIn,omitempty"`
	// Maximum outgoing bandwidth from the public network, measured in Mbps (Mega bit per second). The value range for PayByBandwidth is [0,1024].
	// +kubebuilder:validation:Optional
	InternetMaxBandwidthOut *float64 `json:"internetMaxBandwidthOut,omitempty"`
	// … 如果需要，还可以加更多字段，如 RAM Role、磁盘配置 …
	SystemDiskCategory *string  `json:"systemDiskCategory,omitempty"`
	SystemDiskSize     *float64 `json:"systemDiskSize,omitempty"`
}

// 伸缩组字段
type AliyunScalingGroupSpec struct {
	// Name shown for the scaling group, which must contain 2-64 characters (English or Chinese), starting with numbers, English letters or Chinese characters, and can contain numbers, underscores _, hyphens -, and decimal points .. If this parameter is not specified, the default value is ScalingGroupId.
	// +kubebuilder:validation:Required
	ScalingGroupName string `json:"scalingGroupName,omitempty"`
	// +kubebuilder:validation:Min=0
	MinSize int `json:"minSize"`
	// +kubebuilder:validation:Minimum=1
	MaxSize int `json:"maxSize"`
	// +kubebuilder:validation:Minimum=0
	DesiredCapacity int `json:"desiredCapacity"`
	// vswitch 可以和配置里的 vSwitch 对应，也可独立
	// +kubebuilder:validation:MinItems=1
	VSwitchIDs []string `json:"vswitchIds"`
	// 可选：要挂载的负载均衡实例
	// +optional
	LoadBalancerIDs []string `json:"loadBalancerIds,omitempty"`
	// … 还可以加伸缩策略、通知配置等 …
}

func init() {
	SchemeBuilder.Register(&AliyunMachinePool{}, &AliyunMachinePoolList{})
}
