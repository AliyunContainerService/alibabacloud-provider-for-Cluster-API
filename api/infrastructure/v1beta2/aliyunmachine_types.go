package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

type AliyunMachineStatus struct {
	// 标准 CAPI Machine 的地址字段
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`
	// 一般把 ProviderID 也写到 status，便于观测
	ProviderID string `json:"providerID,omitempty"`
	// 就绪条件等（可按需扩展更多自定义条件）
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=aliyunmachines,scope=Namespaced,shortName=alim;alimc,categories=cluster-api
// +kubebuilder:printcolumn:name="READY",type=string,JSONPath=".status.conditions[?(@.type=='Ready')].status",priority=0
// +kubebuilder:printcolumn:name="PROVIDERID",type=string,JSONPath=".status.providerID",priority=1
// +kubebuilder:printcolumn:name="AGE",type=date,JSONPath=".metadata.creationTimestamp"
type AliyunMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AliyunMachineSpec   `json:"spec,omitempty"`
	Status AliyunMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type AliyunMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AliyunMachine `json:"items"`
}

var (
	AliyunMachineKind             = "AliyunMachine"
	AliyunMachineGroupKind        = schema.GroupKind{Group: "infrastructure.cluster.x-k8s.io", Kind: AliyunMachineKind}.String()
	AliyunMachineKindAPIVersion   = AliyunMachineKind + "." + "v1beta2"
	AliyunMachineGroupVersionKind = schema.GroupVersion{Group: "infrastructure.cluster.x-k8s.io", Version: "v1beta2"}.WithKind(AliyunMachineKind)
)

func init() {
	SchemeBuilder.Register(&AliyunMachine{}, &AliyunMachineList{})
}
