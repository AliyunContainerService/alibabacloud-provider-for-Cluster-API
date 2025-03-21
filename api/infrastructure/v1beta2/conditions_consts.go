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
