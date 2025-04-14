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

package controlplane

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/secret"
	ctrl "sigs.k8s.io/controller-runtime"

	controlplanev1beta2 "cluster-api-provider-aliyun/api/controlplane/v1beta2"
	csv1alpha1 "cluster-api-provider-aliyun/api/cs/v1alpha1"
)

// reconcileKubeconfig 创建内容为 kubeconfig 的 secret 资源, 并设置 control plane 资源的 .Status.Initialized 字段.
func (r *AliyunManagedControlPlaneReconciler) reconcileKubeconfig(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	controlPlane *controlplanev1beta2.AliyunManagedControlPlane,
	managedKubernetes *csv1alpha1.ManagedKubernetes,
) (err error) {
	log := ctrl.LoggerFrom(ctx).WithValues("cluster", cluster.Name)
	log.V(5).Info("Reconciling kubeconfigs for cluster")

	ca := csv1alpha1.CertificateAuthority{}
	if err := ca.Decode(managedKubernetes.Status.AtProvider.CertificateAuthority); err != nil {
		log.Error(err, "CertificateAuthority is isvalid")
		return err
	}
	clusterKey := types.NamespacedName{
		Name:      cluster.Name,
		Namespace: cluster.Namespace,
	}

	// Create the kubeconfig used by CAPI
	configSecret, err := secret.GetFromNamespacedName(ctx, r.Client, clusterKey, secret.Kubeconfig)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrap(err, "failed to get kubeconfig secret")
		}
		// 不存在则创建
		if createErr := r.createCAPIKubeconfigSecret(
			ctx, cluster, ca, controlPlane,
		); createErr != nil {
			return fmt.Errorf("creating kubeconfig secret: %w", createErr)
		}
	} else if updateErr := r.updateCAPIKubeconfigSecret(
		ctx, configSecret, cluster, ca,
	); updateErr != nil {
		return fmt.Errorf("updating kubeconfig secret: %w", updateErr)
	}

	// Set initialized to true to indicate the kubconfig has been created
	controlPlane.Status.Initialized = true
	log.V(5).Info("Successfully reconciled kubeconfigs")
	return nil
}

func (r *AliyunManagedControlPlaneReconciler) createCAPIKubeconfigSecret(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	ca csv1alpha1.CertificateAuthority,
	controlPlane *controlplanev1beta2.AliyunManagedControlPlane,
) error {
	controllerOwnerRef := *metav1.NewControllerRef(
		controlPlane, controlplanev1beta2.GroupVersion.WithKind("AliyunManagedControlPlane"),
	)

	userName := r.getKubeConfigUserName(cluster.Name, false)
	contextName := fmt.Sprintf("%s@%s", userName, cluster.Name)
	cfg := &api.Config{
		APIVersion: api.SchemeGroupVersion.Version,
		Clusters: map[string]*api.Cluster{
			cluster.Name: {
				Server: fmt.Sprintf(
					"https://%s:%d", controlPlane.Spec.ControlPlaneEndpoint.Host,
					controlPlane.Spec.ControlPlaneEndpoint.Port,
				),
				CertificateAuthorityData: ca.ClusterCert,
			},
		},
		Contexts: map[string]*api.Context{
			contextName: {
				Cluster:  cluster.Name,
				AuthInfo: userName,
			},
		},
		CurrentContext: contextName,
		AuthInfos: map[string]*api.AuthInfo{
			userName: {
				ClientCertificateData: ca.ClientCert,
				ClientKeyData:         ca.ClientKey,
			},
		},
	}

	out, err := clientcmd.Write(*cfg)
	if err != nil {
		return errors.Wrap(err, "failed to serialize config to yaml")
	}
	clusterKey := types.NamespacedName{
		Name:      cluster.Name,
		Namespace: cluster.Namespace,
	}

	kubeconfigSecret := kubeconfig.GenerateSecretWithOwner(clusterKey, out, controllerOwnerRef)
	if err := r.Client.Create(ctx, kubeconfigSecret); err != nil {
		return errors.Wrap(err, "failed to create kubeconfig secret")
	}

	return nil
}

func (r *AliyunManagedControlPlaneReconciler) updateCAPIKubeconfigSecret(
	ctx context.Context, configSecret *corev1.Secret,
	cluster *clusterv1.Cluster,
	ca csv1alpha1.CertificateAuthority,
) error {
	log := ctrl.LoggerFrom(ctx)

	log.Info("Updating kubeconfigs for cluster", "cluster-name", cluster.Name)

	data, ok := configSecret.Data[secret.KubeconfigDataName]
	if !ok {
		return errors.Errorf("missing key %q in secret data", secret.KubeconfigDataName)
	}

	config, err := clientcmd.Load(data)
	if err != nil {
		return errors.Wrap(err, "failed to convert kubeconfig Secret into a clientcmdapi.Config")
	}

	userName := r.getKubeConfigUserName(cluster.Name, false)
	config.AuthInfos[userName].ClientCertificateData = ca.ClientCert
	config.AuthInfos[userName].ClientKeyData = ca.ClientKey

	out, err := clientcmd.Write(*config)
	if err != nil {
		return errors.Wrap(err, "failed to serialize config to yaml")
	}

	configSecret.Data[secret.KubeconfigDataName] = out

	err = r.Client.Update(ctx, configSecret)
	if err != nil {
		return fmt.Errorf("updating kubeconfig secret: %w", err)
	}

	return nil
}

func (r *AliyunManagedControlPlaneReconciler) getKubeConfigUserName(
	clusterName string, isUser bool,
) string {
	if isUser {
		return fmt.Sprintf("%s-user", clusterName)
	}

	return fmt.Sprintf("%s-capi-admin", clusterName)
}
