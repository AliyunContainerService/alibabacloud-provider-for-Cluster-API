//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1beta2

import (
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/common"
	"github.com/AliyunContainerService/alibabacloud-provider-for-Cluster-API/api/cs/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/api/v1beta1"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedCluster) DeepCopyInto(out *AliyunManagedCluster) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedCluster.
func (in *AliyunManagedCluster) DeepCopy() *AliyunManagedCluster {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedCluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AliyunManagedCluster) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedClusterList) DeepCopyInto(out *AliyunManagedClusterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]AliyunManagedCluster, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedClusterList.
func (in *AliyunManagedClusterList) DeepCopy() *AliyunManagedClusterList {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedClusterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AliyunManagedClusterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedClusterSpec) DeepCopyInto(out *AliyunManagedClusterSpec) {
	*out = *in
	out.ControlPlaneEndpoint = in.ControlPlaneEndpoint
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedClusterSpec.
func (in *AliyunManagedClusterSpec) DeepCopy() *AliyunManagedClusterSpec {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedClusterSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedClusterStatus) DeepCopyInto(out *AliyunManagedClusterStatus) {
	*out = *in
	if in.FailureDomains != nil {
		in, out := &in.FailureDomains, &out.FailureDomains
		*out = make(v1beta1.FailureDomains, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedClusterStatus.
func (in *AliyunManagedClusterStatus) DeepCopy() *AliyunManagedClusterStatus {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedClusterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedMachinePool) DeepCopyInto(out *AliyunManagedMachinePool) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedMachinePool.
func (in *AliyunManagedMachinePool) DeepCopy() *AliyunManagedMachinePool {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedMachinePool)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AliyunManagedMachinePool) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedMachinePoolList) DeepCopyInto(out *AliyunManagedMachinePoolList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]AliyunManagedMachinePool, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedMachinePoolList.
func (in *AliyunManagedMachinePoolList) DeepCopy() *AliyunManagedMachinePoolList {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedMachinePoolList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AliyunManagedMachinePoolList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedMachinePoolSpec) DeepCopyInto(out *AliyunManagedMachinePoolSpec) {
	*out = *in
	in.ScalingGroup.DeepCopyInto(&out.ScalingGroup)
	if in.ProviderIDList != nil {
		in, out := &in.ProviderIDList, &out.ProviderIDList
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedMachinePoolSpec.
func (in *AliyunManagedMachinePoolSpec) DeepCopy() *AliyunManagedMachinePoolSpec {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedMachinePoolSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedMachinePoolSpecKubernetesConfig) DeepCopyInto(out *AliyunManagedMachinePoolSpecKubernetesConfig) {
	*out = *in
	if in.Tags != nil {
		in, out := &in.Tags, &out.Tags
		*out = make(map[string]*string, len(*in))
		for key, val := range *in {
			var outVal *string
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = new(string)
				**out = **in
			}
			(*out)[key] = outVal
		}
	}
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make([]v1alpha1.LabelsParameters, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Taints != nil {
		in, out := &in.Taints, &out.Taints
		*out = make([]v1alpha1.TaintsParameters, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedMachinePoolSpecKubernetesConfig.
func (in *AliyunManagedMachinePoolSpecKubernetesConfig) DeepCopy() *AliyunManagedMachinePoolSpecKubernetesConfig {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedMachinePoolSpecKubernetesConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedMachinePoolSpecLabel) DeepCopyInto(out *AliyunManagedMachinePoolSpecLabel) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedMachinePoolSpecLabel.
func (in *AliyunManagedMachinePoolSpecLabel) DeepCopy() *AliyunManagedMachinePoolSpecLabel {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedMachinePoolSpecLabel)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedMachinePoolSpecScalingGroup) DeepCopyInto(out *AliyunManagedMachinePoolSpecScalingGroup) {
	*out = *in
	if in.VSwitches != nil {
		in, out := &in.VSwitches, &out.VSwitches
		*out = make(common.VSwitches, len(*in))
		copy(*out, *in)
	}
	if in.InstanceTypes != nil {
		in, out := &in.InstanceTypes, &out.InstanceTypes
		*out = make([]*string, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(string)
				**out = **in
			}
		}
	}
	if in.DesiredSize != nil {
		in, out := &in.DesiredSize, &out.DesiredSize
		*out = new(float64)
		**out = **in
	}
	if in.SystemDiskSize != nil {
		in, out := &in.SystemDiskSize, &out.SystemDiskSize
		*out = new(float64)
		**out = **in
	}
	if in.DataDisks != nil {
		in, out := &in.DataDisks, &out.DataDisks
		*out = make([]v1alpha1.DataDisksParameters, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.SecurityGroupIDs != nil {
		in, out := &in.SecurityGroupIDs, &out.SecurityGroupIDs
		*out = make([]*string, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = new(string)
				**out = **in
			}
		}
	}
	in.KubernetesConfig.DeepCopyInto(&out.KubernetesConfig)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedMachinePoolSpecScalingGroup.
func (in *AliyunManagedMachinePoolSpecScalingGroup) DeepCopy() *AliyunManagedMachinePoolSpecScalingGroup {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedMachinePoolSpecScalingGroup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedMachinePoolSpecTaints) DeepCopyInto(out *AliyunManagedMachinePoolSpecTaints) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedMachinePoolSpecTaints.
func (in *AliyunManagedMachinePoolSpecTaints) DeepCopy() *AliyunManagedMachinePoolSpecTaints {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedMachinePoolSpecTaints)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedMachinePoolStatus) DeepCopyInto(out *AliyunManagedMachinePoolStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(v1beta1.Conditions, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.NodeStatus != nil {
		in, out := &in.NodeStatus, &out.NodeStatus
		*out = new(AliyunManagedMachinePoolStatusNode)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedMachinePoolStatus.
func (in *AliyunManagedMachinePoolStatus) DeepCopy() *AliyunManagedMachinePoolStatus {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedMachinePoolStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AliyunManagedMachinePoolStatusNode) DeepCopyInto(out *AliyunManagedMachinePoolStatusNode) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AliyunManagedMachinePoolStatusNode.
func (in *AliyunManagedMachinePoolStatusNode) DeepCopy() *AliyunManagedMachinePoolStatusNode {
	if in == nil {
		return nil
	}
	out := new(AliyunManagedMachinePoolStatusNode)
	in.DeepCopyInto(out)
	return out
}
