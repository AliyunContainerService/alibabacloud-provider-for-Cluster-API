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

package v1beta2

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

const (
	// ManagedControlPlaneFinalizer allows the controller to clean up resources on delete.
	ManagedControlPlaneFinalizer = "aliyunmanagedcontrolplane.controlplane.cluster.x-k8s.io"
)

const (
	ManagedKubernetesReconcileReadyCondition clusterv1.ConditionType = "ManagedKubernetesReconcileReady"
	ManagedKubernetesReconcileFailedReason   string                  = "ManagedKubernetesReconcileFailed"
	ProviderConfigReconcileReadyCondition    clusterv1.ConditionType = "ProviderConfigReconcileReady"
	ProviderConfigReconcileFailedReason      string                  = "ProviderConfigReconcileFailed"
	VPCReconcileReadyCondition               clusterv1.ConditionType = "VPCReconcileReady"
	VPCReconcileFailedReason                 string                  = "VPCReconcileFailed"
	VSwitchReconcileReadyCondition           clusterv1.ConditionType = "VSwitchReconcileReady"
	VSwitchReconcileFailedReason             string                  = "VSwitchReconcileFailed"
)

const (
	AliyunManagedControlPlaneReadyCondition    clusterv1.ConditionType = "AliyunManagedControlPlaneReady"
	AliyunManagedControlPlaneCreatingCondition clusterv1.ConditionType = "AliyunManagedControlPlaneCreating"
	AliyunManagedControlPlaneUpdatingCondition clusterv1.ConditionType = "AliyunManagedControlPlaneUpdating"
	AliyunManagedControlPlaneFailedReason      string                  = "AliyunManagedControlPlaneFailed"
)
