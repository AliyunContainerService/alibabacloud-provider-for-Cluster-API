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
