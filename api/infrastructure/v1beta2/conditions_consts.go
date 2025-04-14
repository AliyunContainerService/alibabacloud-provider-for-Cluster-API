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
	ManagedMachinePoolFinalizer = "aliyunmanagedmachinepools.infrastructure.cluster.x-k8s.io"
)

const (
	KubernetesNodePoolReconcileReadyCondition clusterv1.ConditionType = "KubernetesNodePoolReconcileReady"
	KubernetesNodePoolReconcileFailedReason   string                  = "KubernetesNodePoolReconcileFailed"
	VSwitchReconcileReadyCondition            clusterv1.ConditionType = "VSwitchReconcileReady"
	VSwitchReconcileFailedReason              string                  = "VSwitchReconcileFailed"
)

const (
	AliyunManagedControlPlaneReadyCondition          clusterv1.ConditionType = "AliyunManagedControlPlaneReady"
	AliyunManagedMachinePoolReadyCondition           clusterv1.ConditionType = "AliyunManagedMachinePoolReady"
	AliyunManagedMachinePoolCreatingCondition        clusterv1.ConditionType = "AliyunManagedMachinePoolCreating"
	AliyunManagedMachinePoolUpdatingCondition        clusterv1.ConditionType = "AliyunManagedMachinePoolUpdating"
	AliyunManagedMachinePoolFailedReason             string                  = "AliyunManagedMachinePoolFailed"
	WaitingForKubernetesNodePoolReconcileReadyReason string                  = "WaitingForKubernetesNodePoolReconcileReady"
	WaitingForAliyunManagedControlPlaneReason        string                  = "WaitingForAliyunManagedControlPlane"
	SetupSDKClientFailedReason                       string                  = "SetupSDKClientFailed"
)
