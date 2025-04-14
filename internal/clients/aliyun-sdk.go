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

package clients

import (
	"encoding/json"
	"io"
	"net/http"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	ess20220222 "github.com/alibabacloud-go/ess-20220222/v2/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/pkg/errors"
)

var (
	essEndpointURL  string = "https://api.aliyun.com/meta/v1/products/Ess/endpoints.json"
	AliyunCreds     aliyunCreds
	EssEndpointList EssEndpointRespInfoList
)

type aliyunCreds struct {
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Region    string `json:"region"`
}

// 每个区域的 endpoint 需要匹配, 否则无法查询到对应的资源信息.
// 展示页: https://api.aliyun.com/product/Ess
// API: https://api.aliyun.com/meta/v1/products/Ess/endpoints.json
type EssEndpointResp struct {
	Code int                 `json:"code"`
	Data EssEndpointRespData `json:"data"`
}
type EssEndpointRespData struct {
	Type      string                  `json:"type"`
	Endpoints EssEndpointRespInfoList `json:"endpoints"`
}
type EssEndpointRespInfoList []EssEndpointRespInfo
type EssEndpointRespInfo struct {
	RegionID   string `json:"regionId"`
	RegionName string `json:"regionName"`
	AreaId     string `json:"areaId"`
	AreaName   string `json:"areaName"`
	Public     string `json:"public"`
	VPC        string `json:"vpc"`
}

func (r EssEndpointRespInfoList) Init() (err error) {
	res, err := http.Get(essEndpointURL)
	if err != nil {
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return
	}

	var essEndpointResp EssEndpointResp
	err = json.Unmarshal(body, &essEndpointResp)
	if err != nil {
		return
	}
	EssEndpointList = essEndpointResp.Data.Endpoints
	if EssEndpointList.Empty() {
		return errors.New("Empty Endpoint List")
	}
	return
}

func (r EssEndpointRespInfoList) Empty() bool {
	return len(r) == 0
}

func (r EssEndpointRespInfoList) Get(regionID string) (string, error) {
	for _, endpointInfo := range r {
		if endpointInfo.RegionID == regionID {
			return endpointInfo.Public, nil
		}
	}
	return "", errors.New("Not Found")
}

/**
 * 使用AK&SK初始化账号Client
 * @param accessKeyId
 * @param accessKeySecret
 * @return Client
 * @throws Exception
 */
func CreateSDKClient(regionID string) (result *ess20220222.Client, err error) {
	// 工程代码泄露可能会导致 AccessKey 泄露，并威胁账号下所有资源的安全性。以下代码示例仅供参考。
	// 建议使用更安全的 STS 方式，更多鉴权访问方式请参见：https://help.aliyun.com/document_detail/378661.html。
	config := &openapi.Config{
		// 必填，请确保代码运行环境设置了环境变量 ALIBABA_CLOUD_ACCESS_KEY_ID。
		AccessKeyId: tea.String(AliyunCreds.AccessKey),
		// 必填，请确保代码运行环境设置了环境变量 ALIBABA_CLOUD_ACCESS_KEY_SECRET。
		AccessKeySecret: tea.String(AliyunCreds.SecretKey),
	}
	endpoint, err := EssEndpointList.Get(regionID)
	if err != nil {
		return
	}
	// Endpoint 请参考 https://api.aliyun.com/product/Ess
	config.Endpoint = tea.String(endpoint)
	result = &ess20220222.Client{}
	result, err = ess20220222.NewClient(config)
	return result, err
}
