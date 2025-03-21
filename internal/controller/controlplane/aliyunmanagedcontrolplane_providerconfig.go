package controlplane

import (
	"context"
	"encoding/json"
	"fmt"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	alibabacloudv1beta1 "cluster-api-provider-aliyun/api/alibabacloud/v1beta1"
	controlplanev1beta2 "cluster-api-provider-aliyun/api/controlplane/v1beta2"
	"cluster-api-provider-aliyun/internal/clients"
)

type CredentialSecret struct {
	Namespace string
	Name      string
	Key       string // secret 中的字段名称

	// json tag 与 internal/clients/alibabacloud.go 中 KeyAccessKey 等变量值保持一致
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Region    string `json:"region"`
}

func (c *CredentialSecret) Decode(secret *corev1.Secret) (err error) {
	data, ok := secret.Data[c.Key]
	if !ok {
		return fmt.Errorf("secret field not found: %s", c.Key)
	}
	// 不需要 base64 解密
	// var data []byte
	// _, err = base64.StdEncoding.Decode(data, dataEncrypted)
	// if err != nil {
	// 	return fmt.Errorf("secret field base64 decoding failed: %w", err)
	// }
	if err = json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("secret field parsed failed: %s", err)
	}

	if c.AccessKey == "" {
		return fmt.Errorf("empty field access_key")
	}
	if c.SecretKey == "" {
		return fmt.Errorf("empty field secret_key")
	}
	// 用户不需要在 secret 中提供 region 信息
	// if c.Region == "" {
	// 	return fmt.Errorf("empty field region")
	// }

	return
}

// reconcileProviderConfig 查找目标 control plane 所在 region 对应的 ProviderConfig 对象,
// 如不存在则创建, 同时还要创建对应的 secret.
func (r *AliyunManagedControlPlaneReconciler) reconcileProviderConfig(
	ctx context.Context,
	log logr.Logger,
	aliyunControlPlane *controlplanev1beta2.AliyunManagedControlPlane,
) (pc *alibabacloudv1beta1.ProviderConfig, err error) {
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Namespace: r.CredentialSecret.Namespace,
		Name:      fmt.Sprintf("aliyun-%s", aliyunControlPlane.Spec.Region),
	}
	if err = r.Client.Get(ctx, secretKey, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			return
		}
		log.Info("try to create Secret")
		credMap := map[string]string{
			clients.KeyAccessKey: r.CredentialSecret.AccessKey,
			clients.KeySecretKey: r.CredentialSecret.SecretKey,
			clients.KeyRegion:    aliyunControlPlane.Spec.Region,
		}
		cred, err := json.Marshal(credMap)
		if err != nil {
			return nil, err
		}
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretKey.Name,
				Namespace: secretKey.Namespace,
				Labels:    map[string]string{},
			},
			Data: map[string][]byte{
				r.CredentialSecret.Key: cred,
			},
		}
		if err = r.Client.Create(ctx, secret, &client.CreateOptions{}); err != nil {
			return nil, err
		}
	}

	pc = &alibabacloudv1beta1.ProviderConfig{}
	pcKey := types.NamespacedName{
		Name: aliyunControlPlane.Spec.Region,
	}
	if err = r.Client.Get(ctx, pcKey, pc); err == nil {
		return
	}
	if !apierrors.IsNotFound(err) {
		return
	}
	log.Info("try to create ProviderConfig")

	pc = &alibabacloudv1beta1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:   aliyunControlPlane.Spec.Region,
			Labels: map[string]string{},
		},
		Spec: alibabacloudv1beta1.ProviderConfigSpec{
			Credentials: alibabacloudv1beta1.ProviderCredentials{
				Source: xpv1.CredentialsSourceSecret,
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{
						SecretReference: xpv1.SecretReference{
							Namespace: secretKey.Namespace,
							Name:      secretKey.Name,
						},
						Key: r.CredentialSecret.Key, // secret 中的字段名称
					},
				},
			},
		},
	}
	if err = r.Client.Create(ctx, pc, &client.CreateOptions{}); err != nil {
		return
	}
	return
}
