# cluster-api-provider-aliyun

本项目基于[Terraform](https://developer.hashicorp.com/terraform) 工具，遵循[Cluster API](https://cluster-api.sigs.k8s.io/introduction) 规范，并参考了[AWS provider](https://cluster-api-aws.sigs.k8s.io/) 项目，在阿里云平台上实现了创建和删除 ACK 托管集群的功能。

该项目支持自定义 ACK 集群配置，包括节点数量、机型等参数的设置。用户可以根据实际需求，灵活配置 ACK 集群的规模和性能。

## 开发环境

- go: 1.22.3
- kubernetes: v1.25.1
- kubebuilder: 3.14.0
- terraform: 1.7.3
- terraform-aliyun-provider: 1.223.2
- clusterctl: v1.6.3
- clusterawsadm: v2.3.1

> 开发环境配置请参考项目根目录[Dockerfile](./Dockerfile)

### 代码编译

```
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/manager cmd/main.go
```

可得到二进制可执行文件, 可自行部署.

## 部署方式

在集群中创建CRD

```
kubectl apply -f config/crd/bases/
```

准备阿里云平台认证信息(`access_key`, `secret_key`), 填入`config/manager/provider_config.yaml`文件.

**必填, 否则程序会启动失败.**

```yaml
  credentials: |
    {
      "access_key": "",
      "secret_key": "",
    }
```

镜像构建(含代码编译)

```
make docker-build
```

以 Deployment 形式部署 manager.

```
make deploy
```

> 会自动完成 Namespace, RBAC, Webhook, Deployment 等资源的构建.

```
[root@k8s-master-01 cluster-api-provider-aliyun]# kubectl get pod
NAME                                                              READY   STATUS    RESTARTS   AGE
cluster-api-provider-aliyun-controller-manager-549c649467-h9sfq   2/2     Running   0          125m
```

## 运行成果

创建测试集群.

```
kubectl apply -f config/samples/ack-test.yaml
```

查询集群状态.

```
[root@k8s-master-01 ~]# kwd cluster
NAME              CLUSTERCLASS   PHASE         AGE   VERSION
ack-test                         Provisioned   16m
```

由于实现了部分 cluster-api 接口, 通过该项目部署的 ACK 集群, 可以通过`clusterctl`获取集群状态, 与`kubectl get cluster`的结果一致.

```
[root@k8s-master-01 ~]# clusterctl describe cluster ack-test
NAME                                                               READY  SEVERITY  REASON  SINCE  MESSAGE
Cluster/ack-test                                                   True                     2m56s
├─ClusterInfrastructure - AliyunManagedCluster/ack-test
├─ControlPlane - AliyunManagedControlPlane/ack-test-control-plane
└─Workers
  └─MachinePool/ack-test-pool-0                                    True                     2m11s
    └─MachinePoolInfrastructure - AliyunManagedMachinePool/ack-test-pool-0

$ clusterctl get kubeconfig ack-test
apiVersion: v1
clusters:
- cluster: {}
## 省略
```
