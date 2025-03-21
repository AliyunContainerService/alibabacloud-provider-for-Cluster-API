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

	commonapi "cluster-api-provider-aliyun/api/common"
	csv1alpha1 "cluster-api-provider-aliyun/api/cs/v1alpha1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AliyunManagedControlPlaneSpec defines the desired state of AliyunManagedControlPlane
type AliyunManagedControlPlaneSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	ClusterName        string                               `json:"clusterName,omitempty"`        // 集群名称
	Region             string                               `json:"region,omitempty"`             // 所属区域(如 cn-hangzhou), 必填, todo: region 是 upjet secret 中声明的
	Version            string                               `json:"version,omitempty"`            // kube版本(如 1.28.3-aliyun.1), 可为空, 空值时使用最新版本
	ClusterSpec        string                               `json:"clusterSpec,omitempty"`        // 集群节点规格(如 ack.pro.small)
	ClusterDomain      string                               `json:"clusterDomain,omitempty"`      // upjet 字段赋值时可为空, 空值时默认使用 cluster.local
	ResourceGroup      string                               `json:"resourceGroup,omitempty"`      //
	DeletionProtection bool                                 `json:"deletionProtection,omitempty"` //
	TimeZone           string                               `json:"timeZone,omitempty"`           // 时区(如 Asia/Shanghai)
	AdditionalTags     AdditionalTags                       `json:"additionalTags,omitempty"`
	Network            AliyunManagedControlPlaneSpecNetwork `json:"network,omitempty"`

	Addons  []csv1alpha1.AddonsParameters        `json:"addons,omitempty"`  //
	Logging AliyunManagedControlPlaneSpecLogging `json:"logging,omitempty"` //
	// todo: terway 需要设置 pod_vswitch_ids + addon(eniip), flannel 需要设置 pod_cidr + addon(flannel)
	CNI              AliyunManagedControlPlaneSpecCNI              `json:"cni,omitempty"`
	EntryptionConfig AliyunManagedControlPlaneSpecEncryptionConfig `json:"entryptionConfig,omitempty"` //
	KubeProxy        AliyunManagedControlPlaneSpecKubeProxy        `json:"kubeProxy,omitempty"`        //
	EndpointAccess   AliyunManagedControlPlaneSpecEndpointAccess   `json:"endpointAccess,omitempty"`   // todo: 是否为 apiserver 配置公网IP

	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

type AliyunManagedControlPlaneSpecNetwork struct {
	Vpc           commonapi.VPC                              `json:"vpc,omitempty"`
	VSwitches     commonapi.VSwitches                        `json:"vSwitches,omitempty"`
	PodCIDR       string                                     `json:"podCIDR,omitempty"`       // todo: 新增
	ServiceCIDR   string                                     `json:"serviceCIDR,omitempty"`   // 必填, 点分十进制IP, 带掩码(如 172.16.0.0/16)
	NatGateway    bool                                       `json:"natGateway,omitempty"`    //
	SecurityGroup AliyunManagedControlPlaneSpecSecurityGroup `json:"securityGroup,omitempty"` //
}

type AliyunManagedControlPlaneSpecSecurityGroup struct {
	ID         string `json:"id,omitempty"`
	Create     bool   `json:"create,omitempty"`
	Enterprice bool   `json:"enterprice,omitempty"`
}

type AliyunManagedControlPlaneSpecEndpointAccess struct {
	Public bool `json:"public,omitempty"`
}
type AliyunManagedControlPlaneSpecCNI struct {
	Disable bool `json:"disable,omitempty"`
}
type AliyunManagedControlPlaneSpecEncryptionConfig struct {
	ProviderKey string `json:"providerKey,omitempty"`
}

type AliyunManagedControlPlaneSpecLogging struct {
	Enable     bool      `json:"enable,omitempty"`
	TTL        string    `json:"ttl,omitempty"`
	Components []*string `json:"components,omitempty"` // (如 [apiserver, kcm, ])
}
type AliyunManagedControlPlaneSpecKubeProxy struct {
	ProxyMode string `json:"proxyMode,omitempty"` // (如 ipvs, iptables)
}

// AliyunManagedControlPlaneStatus defines the observed state of AliyunManagedControlPlane
type AliyunManagedControlPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Ready          bool                     `json:"ready,omitempty"`       // 平台创建 control plane 成功(就绪)后, 即可设置为 true
	Initialized    bool                     `json:"initialized,omitempty"` // 集群创建完成且 kube config 配置创建完成后, 即可设置为 true
	FailureDomains clusterv1.FailureDomains `json:"failureDomains,omitempty"`
	FailureReason  string                   `json:"failureReason,omitempty"` // todo(reason 与 message 是否重复了)
	FailureMessage string                   `json:"failureMessage,omitempty"`

	ExternalManagedControlPlane bool                                   `json:"externalManagedControlPlane,omitempty"`
	Version                     string                                 `json:"version,omitempty"` // 貌似与 .spec.version 保持一致
	Conditions                  clusterv1.Conditions                   `json:"conditions,omitempty"`
	NetworkStatus               AliyunManagedControlPlaneStatusNetwork `json:"networkStatus,omitempty"`
	Addons                      []csv1alpha1.AddonsObservation         `json:"addons,omitempty"`
	ClusterID                   string                                 `json:"clusterId,omitempty"` // todo: 新增
}

type AliyunManagedControlPlaneStatusNetwork struct {
	SecurityGroup  string `json:"securityGroup,omitempty"`
	ApiServerSlbID string `json:"apiServerSlbId,omitempty"`
	NatGatewayID   string `json:"natGatewayId,omitempty"`
}

// 自定义信息展示列: 所属 Cluster 名称, Ready 状态.

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.metadata.labels.cluster\.x-k8s\.io/cluster-name`
//+kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
//
// AliyunManagedControlPlane is the Schema for the aliyunmanagedcontrolplanes API
type AliyunManagedControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AliyunManagedControlPlaneSpec   `json:"spec,omitempty"`
	Status AliyunManagedControlPlaneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// AliyunManagedControlPlaneList contains a list of AliyunManagedControlPlane
type AliyunManagedControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AliyunManagedControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AliyunManagedControlPlane{}, &AliyunManagedControlPlaneList{})
}
