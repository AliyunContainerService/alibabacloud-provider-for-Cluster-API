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

	commonapi "cluster-api-provider-aliyun/api/common"
	csv1alpha1 "cluster-api-provider-aliyun/api/cs/v1alpha1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AliyunManagedMachinePoolSpec defines the desired state of AliyunManagedMachinePool
type AliyunManagedMachinePoolSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	ClusterID       string                                   `json:"clusterId,omitempty"`
	AckNodePoolName string                                   `json:"ackNodePoolName,omitempty"`
	ResourceGroupID string                                   `json:"resourceGroupId,omitempty"`
	ScalingGroup    AliyunManagedMachinePoolSpecScalingGroup `json:"scalingGroup,omitempty"`
	ProviderIDList  []string                                 `json:"providerIDList,omitempty"`
	Region          string                                   `json:"region,omitempty"` // 所属区域(如 cn-hangzhou), 必填, todo: region 是 upjet secret 中声明的
}

type AliyunManagedMachinePoolSpecScalingGroup struct {
	VSwitches                  commonapi.VSwitches                          `json:"vSwitches,omitempty"`     // todo(字段类型, 字符串数组), 必填
	InstanceTypes              []*string                                    `json:"instanceTypes,omitempty"` // todo(字段类型, 可选多规格?), 必填
	DesiredSize                *float64                                     `json:"desiredSize,omitempty"`   // 工作节点数量, 必填
	Password                   string                                       `json:"password,omitempty"`
	KeyName                    string                                       `json:"keyName,omitempty"`
	SystemDiskCategory         string                                       `json:"systemDiskCategory,omitempty"`
	SystemDiskSize             *float64                                     `json:"systemDiskSize,omitempty"` // todo(字段类型, 20Gi?)
	SystemDiskPerformanceLevel string                                       `json:"systemDiskPerformanceLevel,omitempty"`
	DataDisks                  []csv1alpha1.DataDisksParameters             `json:"dataDisks,omitempty"`
	ImageType                  string                                       `json:"imageType,omitempty"`
	SecurityGroupIDs           []*string                                    `json:"securityGroupIds,omitempty"` // todo(字段类型?)
	KubernetesConfig           AliyunManagedMachinePoolSpecKubernetesConfig `json:"kubernetesConfig,omitempty"`
}

type AliyunManagedMachinePoolSpecKubernetesConfig struct {
	RuntimeName    string                        `json:"runtimeName,omitempty"`    //
	RuntimeVersion string                        `json:"runtimeVersion,omitempty"` //
	UserData       string                        `json:"userData,omitempty"`       // todo(字段类型)
	Tags           map[string]*string            `json:"tags,omitempty"`           // todo(字段类型)
	Labels         []csv1alpha1.LabelsParameters `json:"labels,omitempty"`         // todo(字段类型, 与 tags 是否重复)
	Taints         []csv1alpha1.TaintsParameters `json:"taints,omitempty"`         // todo(字段类型)
}
type AliyunManagedMachinePoolSpecLabel struct {
	Key   string `json:"key,omitempty"`   //
	Value string `json:"value,omitempty"` //
}
type AliyunManagedMachinePoolSpecTaints struct {
	Key    string `json:"key,omitempty"`    //
	Value  string `json:"value,omitempty"`  //
	Effect string `json:"effect,omitempty"` //
}

// AliyunManagedMachinePoolStatus defines the observed state of AliyunManagedMachinePool
type AliyunManagedMachinePoolStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Ready    bool  `json:"ready,omitempty"` //
	Replicas int32 `json:"replicas"`        // 根据实际启动的节点数量设置, cluster-api 会根据此值设置 MachinePool.Status.Replicas

	AckNodePoolID  string                              `json:"ackNodePoolId,omitempty"`
	State          string                              `json:"state,omitempty"`
	FailureReason  string                              `json:"failureReason,omitempty"` // todo(reason 与 message 是否重复了)
	FailureMessage string                              `json:"failureMessage,omitempty"`
	Conditions     clusterv1.Conditions                `json:"conditions,omitempty"`
	NodeStatus     *AliyunManagedMachinePoolStatusNode `json:"nodeStatus,omitempty"`
}

type AliyunManagedMachinePoolStatusNode struct {
	DesiredNodes      int `json:"desiredNodes,omitempty"`      //
	FailedNodes       int `json:"failedNodes,omitempty"`       //
	HealthyNodes      int `json:"healthyNodes,omitempty"`      //
	InitialNodes      int `json:"initialNodes,omitempty"`      //
	OfflineNodes      int `json:"offlineNodes,omitempty"`      //
	RemovingNodes     int `json:"removingNodes,omitempty"`     //
	RemovingWaitNodes int `json:"removingWaitNodes,omitempty"` //
	ServingNodes      int `json:"servingNodes,omitempty"`      //
	SpotNodes         int `json:"spotNodes,omitempty"`         //
	TotalNodes        int `json:"totalNodes,omitempty"`        //

}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
//+kubebuilder:printcolumn:name="Replicas",type=string,JSONPath=`.status.replicas`

// AliyunManagedMachinePool is the Schema for the aliyunmanagedmachinepools API
type AliyunManagedMachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AliyunManagedMachinePoolSpec   `json:"spec,omitempty"`
	Status AliyunManagedMachinePoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AliyunManagedMachinePoolList contains a list of AliyunManagedMachinePool
type AliyunManagedMachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AliyunManagedMachinePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AliyunManagedMachinePool{}, &AliyunManagedMachinePoolList{})
}
