/*
Copyright 2024.

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

// AliyunManagedClusterSpec defines the desired state of AliyunManagedCluster
type AliyunManagedClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// 该字段必须放在 spec 块下, 见
	// [cluster-api] internal/controllers/cluster/cluster_controller_phases.go
	// Reconciler.reconcileInfrastructure() 函数中, util.UnstructuredUnmarshalField() 的调用处
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

// AliyunManagedClusterStatus defines the observed state of AliyunManagedCluster
type AliyunManagedClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Ready bool `json:"ready,omitempty"`

	FailureDomains clusterv1.FailureDomains `json:"failureDomains,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.metadata.labels.cluster\.x-k8s\.io/cluster-name`
//+kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
//+kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.spec.controlPlaneEndpoint`

// AliyunManagedCluster is the Schema for the aliyunmanagedclusters API
type AliyunManagedCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AliyunManagedClusterSpec   `json:"spec,omitempty"`
	Status AliyunManagedClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AliyunManagedClusterList contains a list of AliyunManagedCluster
type AliyunManagedClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AliyunManagedCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AliyunManagedCluster{}, &AliyunManagedClusterList{})
}
