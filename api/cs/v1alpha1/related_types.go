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

package v1alpha1

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
)

type ManagedKubernetesStatusConnection struct {
	APIServerInternet string `json:"api_server_internet"`
	APIServerIntranet string `json:"api_server_intranet"`
	MasterPublicIP    string `json:"master_public_ip"`
	ServiceDomain     string `json:"service_domain"`

	Host string
	Port int64
}

func (c *ManagedKubernetesStatusConnection) Decode(src map[string]*string) (err error) {
	if src == nil {
		return fmt.Errorf("empty block ManagedKubernetesStatusConnection")
	}
	if _, ok := src["api_server_internet"]; ok {
		c.APIServerInternet = *src["api_server_internet"]
	}
	if _, ok := src["api_server_intranet"]; ok {
		c.APIServerIntranet = *src["api_server_intranet"]
	}
	if _, ok := src["master_public_ip"]; ok {
		c.MasterPublicIP = *src["master_public_ip"]
	}
	if _, ok := src["service_domain"]; ok {
		c.ServiceDomain = *src["service_domain"]
	}
	apiServer := c.APIServerInternet

	if c.APIServerInternet == "" {
		if c.APIServerIntranet == "" {
			return fmt.Errorf("empty field api_server_internet and api_server_intranet")
		}
		apiServer = c.APIServerIntranet
	}

	endpointInfo, err := url.Parse(apiServer)
	if err != nil {
		return fmt.Errorf("failed to parse api_server_internet: %+v", src)
	}

	host, portStr := endpointInfo.Hostname(), endpointInfo.Port()
	if host == "" || portStr == "" {
		return fmt.Errorf("empty host or port: %+v", src)
	}

	port, err := strconv.ParseInt(portStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse port: %+v", src)
	}

	c.Host, c.Port = host, port
	return
}

type CertificateAuthority struct {
	ClusterCertStr string `json:"cluster_cert"`
	ClientCertStr  string `json:"client_cert"`
	ClientKeyStr   string `json:"client_key"`

	ClusterCert []byte
	ClientCert  []byte
	ClientKey   []byte
}

func (c *CertificateAuthority) Decode(src map[string]*string) (err error) {
	if src == nil {
		return fmt.Errorf("empty block CertificateAuthority")
	}
	if _, ok := src["cluster_cert"]; ok {
		c.ClusterCertStr = *src["cluster_cert"]
	}
	if _, ok := src["client_cert"]; ok {
		c.ClientCertStr = *src["client_cert"]
	}
	if _, ok := src["client_key"]; ok {
		c.ClientKeyStr = *src["client_key"]
	}
	if c.ClusterCertStr == "" {
		return fmt.Errorf("empty field cluster_cert")
	}
	if c.ClientCertStr == "" {
		return fmt.Errorf("empty field client_cert")
	}
	if c.ClientKeyStr == "" {
		return fmt.Errorf("empty field client_key")
	}

	c.ClusterCert, err = base64.StdEncoding.DecodeString(c.ClusterCertStr)
	if err != nil {
		return fmt.Errorf("decoding cluster CA cert: %w", err)
	}
	c.ClientCert, err = base64.StdEncoding.DecodeString(c.ClientCertStr)
	if err != nil {
		return fmt.Errorf("decoding cluster CA cert: %w", err)
	}
	c.ClientKey, err = base64.StdEncoding.DecodeString(c.ClientKeyStr)
	if err != nil {
		return fmt.Errorf("decoding cluster CA cert: %w", err)
	}

	return
}
