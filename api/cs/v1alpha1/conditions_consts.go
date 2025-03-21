package v1alpha1

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

const (
	CrossPlaneReadyCondition     clusterv1.ConditionType = "CrossPlaneReady"
	CrossPlaneFailedCondition    string                  = "CrossPlaneFailed"
	UpjetProviderReadyCondition  clusterv1.ConditionType = "UpjetProviderReady"
	UpjetProviderFailedCondition string                  = "UpjetProviderFailed"
)
