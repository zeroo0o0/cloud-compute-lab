# Lab4：自建 Kubernetes 游戏部署实验

本实验要求你把一个分布式文字游戏部署到自己配置的 Kubernetes 集群上，并让它具备自动扩缩容、基础异常恢复、状态一致性和较好的资源利用率。

---

**【重要评分导向】**

**本实验不鼓励简单堆叠资源。功能正确、服务稳定是基础；在此前提下，花费越少、实验得分越高。**

---

实验提供游戏源码和测试脚本。你需要自己完成 Dockerfile、Kubernetes YAML、镜像构建、镜像分发、部署验证和性能调优。

为了避免混淆，本文统一使用下面几个说法：

| 名称 | 含义 |
| --- | --- |
| 本机 | 你自己的电脑，例如 Windows、macOS 或 Linux 笔记本 |
| 主节点 | Kubernetes control-plane 节点，通常可以从本机远程登录 |
| worker 节点 | Kubernetes 工作节点，通常只有内网 IP，Pod 主要运行在这些节点上 |
| 集群节点 | 主节点和 worker 节点的统称 |

## 1. 实验目标

最终你需要在自己的 Kubernetes 集群中部署出下面这套服务：

```text
命名空间：lab4
外部入口：<可访问的节点地址>:30910

业务组件：
lab4-gateway
lab4-coordinator
lab4-map-green
lab4-map-cave
lab4-map-ruins

状态组件：
lab4-redis

弹性伸缩：
5 个业务 Deployment 都需要配置 HPA
HPA minReplicas=1
HPA maxReplicas=10
Redis 不需要配置 HPA
```

部署完成后，客户端应该可以通过下面的地址连接游戏：

```text
<节点地址>:30910
```

例如：

```text
203.0.113.10:30910
```

## 2. 前置资料

如果你还没有完成租服务器、安装 Docker 或配置 Kubernetes 集群，请先参考：

- [Docker 上云实战](./Docker上云实战.pptx)
- [K8s 使用](./k8s使用.pptx)
- [阿里云 K8s 集群搭建指南](./阿里云%20K8s%20集群搭建指南.md)

其中 [阿里云 K8s 集群搭建指南](./阿里云%20K8s%20集群搭建指南.md) 是集群搭建指南，metrics-server 的安装或修复也可以参考它。

这几个资料只负责基础环境。本 README 重点说明 Lab4 本身要做什么。

## 3. 可选：配置本机 kubeconfig

kubeconfig 是 `kubectl` 连接 Kubernetes 集群时使用的配置文件。它里面记录了 API Server 地址、证书和访问凭据。

配置本机 kubeconfig 的好处是：你可以直接在自己的电脑上运行 `kubectl get pods`、`kubectl apply -f ...`、`watch ...` 等命令，不用每次都远程登录到主节点。

这一步是可选的。即使你没有配置本机 kubeconfig，也不影响基础测试和评分脚本；测试脚本会在需要时退回远程登录模式，让你输入主节点地址、登录用户名、连接端口等信息。

如果你想在本机直接控制集群，可以按下面流程尝试。

1. 在本机安装 `kubectl`。

2. 从主节点复制 kubeconfig。

Linux/macOS：

```bash
mkdir -p ~/.kube
scp <服务器用户名>@<主节点地址>:/etc/kubernetes/admin.conf ~/.kube/config
chmod 600 ~/.kube/config

mkdir -p ~/.kube
scp root@120.79.200.215:/etc/kubernetes/admin.conf ~/.kube/config
chmod 600 ~/.kube/config
```

Windows PowerShell：

```powershell
mkdir $HOME\.kube
scp <服务器用户名>@<主节点地址>:/etc/kubernetes/admin.conf $HOME\.kube\config
```

3. 打开 kubeconfig，找到 `server:` 这一行。

它可能长这样：

```text
server: https://10.0.1.10:6443
```

如果你的电脑无法访问这个内网 IP，可以把它改成主节点地址：

```text
server: https://<主节点地址>:6443
```

4. 确认云平台网络规则允许你的电脑访问 `6443` 端口。

建议只允许自己的电脑访问，不要对无关地址开放。

5. 在本机测试：

```bash
kubectl get nodes -o wide
kubectl get pods -A
```

如果遇到证书错误、网络不通或云平台网络规则问题，不要在这里卡太久。你可以直接远程登录到主节点运行 `kubectl`，或者让测试脚本使用远程登录模式。本机 kubeconfig 只是为了方便，不是完成实验的必要条件。

## 4. 开始前检查

在正式部署 Lab4 之前，先确认 Kubernetes 集群本身是好的。

如果你已经配置了本机 kubeconfig，可以在本机执行。否则远程登录到主节点执行。

```bash
kubectl get nodes -o wide
kubectl get pods -A
```

你应该看到所有节点都是 `Ready`。

HPA 依赖 metrics-server。请继续检查：

```bash
kubectl top nodes
kubectl top pods -A
```

如果 `kubectl top` 报错，说明 metrics-server 没有安装好，HPA 评分会受影响。metrics-server 的安装或修复请参考 [阿里云 K8s 集群搭建指南](./阿里云%20K8s%20集群搭建指南.md)。

## 5. 分步完成实验

下面按照真正做实验的顺序说明。建议你一步一步完成，不要一上来就直接写一大堆 YAML。

### 第一步：把 Lab4 代码传到主节点

在哪里做：本机。

为什么做：本实验建议在云服务器上构建镜像，不建议在本机电脑构建镜像。这样可以避免本机架构、网络、Docker 环境和服务器不一致带来的问题。

如果你的主节点可以从本机访问，可以直接传：

```bash
scp -r Lab4 <服务器用户名>@<主节点地址>:~/Lab4
scp -r Lab4 <root>@<120.79.200.215>:~/Lab4
```

然后登录主节点：

```bash
ssh <服务器用户名>@<主节点地址>
cd ~/Lab4
```

如果你已经在主节点上下载或解压了 Lab4，只需要进入目录：

```bash
cd ~/Lab4
```

完成后验证：

```bash
ls
```

你应该能看到：

```text
cmd
cloud
cluster
storage
test
go.mod
README.md
```

### 第二步：理解要启动哪些程序

在哪里做：主节点。

为什么做：写 Dockerfile 和 YAML 之前，需要先知道每个容器到底应该启动哪个 Go 程序。

本实验有 3 类业务程序：

```text
gateway       -> ./cmd/cloud-gateway
coordinator  -> ./cmd/cloud-coordinator
map          -> ./cmd/cloud-map
```

其中 map 程序会被部署成 3 个地图服务：

```text
lab4-map-green
lab4-map-cave
lab4-map-ruins
```

它们可以共用同一个 map 镜像，通过环境变量区分地图。

客户端请求的大致路径是：

```text
client
  -> <节点地址>:30910
  -> lab4-gateway Service
  -> gateway Pod
  -> lab4-coordinator Service
  -> coordinator Pod
  -> lab4-map-green / lab4-map-cave / lab4-map-ruins Service
  -> map Pod
  -> Redis
```

各组件职责如下：

| 组件 | 作用 |
| --- | --- |
| gateway | 对外暴露 TCP 游戏入口，维护客户端连接，把玩家操作转发给 coordinator |
| coordinator | 处理登录、玩家会话、地图路由、跨地图操作、交易等全局逻辑 |
| map-green | green 地图服务，处理该地图内移动、拾取、互动等逻辑 |
| map-cave | cave 地图服务 |
| map-ruins | ruins 地图服务 |
| Redis | 保存用户数据、会话数据、地图 checkpoint、leader 租约等状态 |

### 第三步：编写 Dockerfile

在哪里做：主节点的 `~/Lab4` 目录。

为什么做：Kubernetes 运行的是容器，不是直接运行 Go 源码。你需要把 gateway、coordinator、map 分别做成镜像。

建议至少编写 3 个 Dockerfile：

```text
Dockerfile.gateway
Dockerfile.coordinator
Dockerfile.map
```

推荐使用多阶段构建：

```text
第一阶段：使用 golang 镜像编译 Go 二进制
第二阶段：使用 alpine、distroless 或 scratch 放入编译好的二进制
```

国内网络直连 Docker Hub 可能不稳定。推荐在 Dockerfile 的构建阶段优先使用下面这个 Go 镜像源：

```text
m.daocloud.io/docker.io/library/golang:1.21-alpine
```

例如你可以在 Dockerfile 中写：

```dockerfile
ARG GO_BUILDER_IMAGE=m.daocloud.io/docker.io/library/golang:1.21-alpine
FROM ${GO_BUILDER_IMAGE} AS builder
```

你需要保证容器启动时执行的是正确入口：

```text
gateway       -> ./cmd/cloud-gateway
coordinator  -> ./cmd/cloud-coordinator
map          -> ./cmd/cloud-map
```

如果主节点安装了 Go，也可以先检查源码：

```bash
go test ./...
```

没有安装 Go 也没关系，只要你的 Dockerfile 能在构建阶段用 Go 镜像完成编译即可。

### 第四步：在主节点构建镜像

在哪里做：主节点的 `~/Lab4` 目录。

为什么做：把源码变成 Kubernetes 可以运行的镜像。本实验建议镜像在服务器上构建，不要求在本机电脑上构建。

示例命令：

```bash
docker build -t lab4-gateway:v1 -f Dockerfile.gateway .
docker build -t lab4-coordinator:v1 -f Dockerfile.coordinator .
docker build -t lab4-map:v1 -f Dockerfile.map .
```

Redis 可以使用 `redis:7-alpine`。国内网络建议先从 DaoCloud 代理源拉取，再 tag 回标准镜像名，这样后面的 `docker save` 和 YAML 可以继续统一写 `redis:7-alpine`：

```bash
docker pull m.daocloud.io/docker.io/library/redis:7-alpine
docker tag m.daocloud.io/docker.io/library/redis:7-alpine redis:7-alpine
```

如果你的服务器能稳定访问 Docker Hub，也可以直接执行 `docker pull redis:7-alpine`。

完成后验证：

```bash
docker images | grep lab4
docker images | grep redis
```

你应该至少能看到：

```text
lab4-gateway:v1
lab4-coordinator:v1
lab4-map:v1
redis:7-alpine
```

### 第五步：把镜像导入 Kubernetes 运行时

在哪里做：主节点，以及所有可能运行 Pod 的 worker 节点。

为什么做：`docker images` 里有镜像，不代表 Kubernetes 一定能用到。很多集群使用 containerd 作为 Kubernetes 运行时，所以需要把镜像导入到 containerd 的 `k8s.io` 命名空间。

先在主节点导出镜像包：

```bash
docker save -o ~/lab4-images.tar lab4-gateway:v1 lab4-coordinator:v1 lab4-map:v1 redis:7-alpine
```

在主节点导入到 containerd：

```bash
ctr -n k8s.io images import ~/lab4-images.tar
```

然后把镜像包分发到每个 worker 节点。注意这里使用的是 worker 内网 IP，因为主节点通常可以访问 worker 内网。

```bash
scp ~/lab4-images.tar <服务器用户名>@<worker内网IP>:~/lab4-images.tar
ssh <服务器用户名>@<worker内网IP> "ctr -n k8s.io images import ~/lab4-images.tar"

scp ~/lab4-images.tar root@10.0.1.25:~/lab4-images.tar
ssh root@10.0.1.25 "ctr -n k8s.io images import ~/lab4-images.tar"
scp ~/lab4-images.tar root@10.0.1.26:~/lab4-images.tar
ssh root@10.0.1.26 "ctr -n k8s.io images import ~/lab4-images.tar"
```

如果你有 3 个 worker 节点，就对 3 个 worker 都执行一次。

完成后可以在每个节点验证：

```bash
ctr -n k8s.io images ls | grep lab4
ctr -n k8s.io images ls | grep redis
```

如果节点安装了 `crictl`，也可以用：

```bash
crictl images | grep lab4
crictl images | grep redis
```

如果你决定使用镜像仓库，也可以把镜像 push 到自己的仓库，然后在 YAML 中写完整镜像地址。镜像仓库相关流程可以参考 [Docker 上云实战](./Docker上云实战.pptx)。本实验 README 默认推荐 `scp + ctr import`，因为它对初学者更直接。

### 第六步：编写 Kubernetes YAML

在哪里做：主节点的 `~/Lab4` 目录。

为什么做：YAML 描述了 Kubernetes 应该创建哪些资源、每个 Pod 用哪个镜像、暴露哪些端口、如何扩缩容。

建议新建一个目录存放 YAML：

```bash
mkdir -p deploy
```

你至少需要编写这些资源：

```text
Namespace
ServiceAccount
Role
RoleBinding
Deployment: gateway
Deployment: coordinator
Deployment: map-green
Deployment: map-cave
Deployment: map-ruins
StatefulSet: redis
Service: gateway
Service: coordinator
Service: map-green
Service: map-cave
Service: map-ruins
Service: redis
HorizontalPodAutoscaler: 5 个业务组件
```

写 YAML 时重点检查这些点：

- 所有资源都放在 `lab4` 命名空间。
- YAML 中的 `image` 名字必须和第五步导入的镜像名一致，例如 `lab4-gateway:v1`。
- 如果使用本地导入镜像，建议设置 `imagePullPolicy: IfNotPresent`。
- gateway 使用 NodePort 暴露 `30910`。
- coordinator、map、redis 使用 ClusterIP，只在集群内部访问。
- 5 个业务 Deployment 都要配置 HPA。
- Redis 使用 StatefulSet，不建议配置 HPA。
- 每个业务容器都要配置 CPU requests，否则 HPA 可能无法计算 CPU 利用率。
- 建议配置 readinessProbe、livenessProbe、preStop 和合理的 `terminationGracePeriodSeconds`。

### 第七步：部署到 Kubernetes 集群

在哪里做：主节点。

为什么做：把你写好的 YAML 应用到集群中。

执行：

```bash
kubectl apply -f deploy/
```

这条命令会按 `deploy/` 目录里的 YAML 原样部署资源。它不会自动替你选择镜像。

如果你的 YAML 里已经写好了真实镜像名，例如：

```text
image: lab4-gateway:v1
image: lab4-coordinator:v1
image: lab4-map:v1
```

并且这些镜像已经导入到所有可能运行 Pod 的节点，那么 `kubectl apply -f deploy/` 就可以直接部署。

如果 YAML 里的 `image` 和你实际构建、导入的镜像名不一致，就需要先修改 YAML，或者部署后再用 `kubectl set image` 更新镜像。

查看资源：

```bash
kubectl -n lab4 get pods -o wide
kubectl -n lab4 get svc
kubectl -n lab4 get hpa -o wide
kubectl -n lab4 get statefulset
```

正常情况下，Pod 应该逐渐变成 `Running`，并且 `READY` 为 `1/1`。

如果有问题，优先看：

```bash
kubectl -n lab4 describe pod <pod-name>
kubectl -n lab4 logs <pod-name>
kubectl -n lab4 get events --sort-by=.lastTimestamp
```

### 第八步：验证游戏入口

在哪里做：本机或主节点都可以。

为什么做：Pod Running 不代表游戏入口一定能访问。你还需要确认 NodePort、云平台网络规则、gateway、coordinator、map、Redis 都串起来了。

先确认 gateway Service：

```bash
kubectl -n lab4 get svc lab4-gateway
```

它应该暴露：

```text
9310:30910/TCP
```

确认云平台网络规则允许访问 `30910` 端口。

然后用 admin 工具检查游戏状态：

```bash
go run ./cmd/admin 状态 <节点地址>:30910

go run ./cmd/admin 状态 120.79.200.215:30910
```

也可以启动交互式客户端：

```bash
go run ./cmd/client <节点地址>:30910


```

如果主节点没有 Go，可以在本机运行这些命令；只要本机能访问 `<节点地址>:30910` 即可。

### 第九步：运行基础测试

在哪里做：本机或主节点都可以。

为什么做：基础测试用于判断你的部署是否成功、功能是否完整。它不追求区分度，主要检查“能不能跑”。

Linux/macOS：

```bash
./test/run-autotest.sh
```

Windows PowerShell：

```powershell
.\test\run-autotest.ps1
```

如果脚本提示：

```text
Gateway address, for example 203.0.113.10:30910:
```

请输入你的游戏入口：

```text
<节点地址>:30910

120.79.200.215:30910
```

如果你的本机已经配置好 kubeconfig，脚本会直接访问 Kubernetes 集群检查资源。如果本机没有 kubeconfig，脚本会退回远程登录模式，提示你输入主节点地址、登录用户名、连接端口等信息。

### 第十步：运行竞技评分并观察 HPA

在哪里做：本机或主节点都可以。

为什么做：竞技评分用来区分不同同学的部署质量和调优效果，满分 100 分。

运行：

```bash
./test/run-scoretest.sh
```

Windows PowerShell：

```powershell
.\test\run-scoretest.ps1
```

调试时可以先跑快速模式：

```bash
LAB4_SCORE_FAST=1 LAB4_SKIP_CHAOS=1 ./test/run-scoretest.sh
```

也可以手动指定游戏入口：

```bash
LAB4_GATEWAY_ADDR=<节点地址>:30910 ./test/run-scoretest.sh
```

评分时建议另开一个终端观察 HPA：

```bash
watch -n 0.5 'kubectl -n lab4 get pods,hpa -o wide'
```

HPA 生效时通常会看到：

```text
TARGETS 接近或超过 CPU 目标值
REPLICAS 从 1 增加到更多
新增 Pod 进入 Running
```

注意：竞技评分会真实触发 HPA 扩容。重复评分前建议等几分钟，让 HPA 缩回稳定状态，否则上一轮测试残留副本可能影响“空闲不误扩”。

## 6. 部署总流程

如果你已经理解上面的每一步，可以按下面的总流程快速回顾。

在本机：

```bash
scp -r Lab4 <服务器用户名>@<主节点地址>:~/Lab4
ssh <服务器用户名>@<主节点地址>
```

在主节点：

```bash
cd ~/Lab4

# 编写 Dockerfile.gateway / Dockerfile.coordinator / Dockerfile.map
docker build -t lab4-gateway:v1 -f Dockerfile.gateway .
docker build -t lab4-coordinator:v1 -f Dockerfile.coordinator .
docker build -t lab4-map:v1 -f Dockerfile.map .
docker pull m.daocloud.io/docker.io/library/redis:7-alpine
docker tag m.daocloud.io/docker.io/library/redis:7-alpine redis:7-alpine

docker save -o ~/lab4-images.tar lab4-gateway:v1 lab4-coordinator:v1 lab4-map:v1 redis:7-alpine
ctr -n k8s.io images import ~/lab4-images.tar

# 对每个 worker 节点执行
scp ~/lab4-images.tar <服务器用户名>@<worker内网IP>:~/lab4-images.tar
ssh <服务器用户名>@<worker内网IP> "ctr -n k8s.io images import ~/lab4-images.tar"

# 编写 deploy/ 下的 Kubernetes YAML
kubectl apply -f deploy/

kubectl -n lab4 get pods,svc,hpa -o wide
```

验证：

```bash
go run ./cmd/admin 状态 <节点地址>:30910
./test/run-autotest.sh
./test/run-scoretest.sh
```

## 7. 资源名和端口要求

测试脚本会按下面的名字查找资源，请保持一致。

Deployment：

```text
lab4-gateway
lab4-coordinator
lab4-map-green
lab4-map-cave
lab4-map-ruins
```

StatefulSet：

```text
lab4-redis
```

Service：

```text
lab4-gateway
lab4-coordinator
lab4-map-green
lab4-map-cave
lab4-map-ruins
lab4-redis
```

HPA：

```text
lab4-gateway
lab4-coordinator
lab4-map-green
lab4-map-cave
lab4-map-ruins
```

服务端口约定：

| 组件 | 端口 | 用途 |
| --- | --- | --- |
| gateway | 9310 | 对外 TCP 游戏端口 |
| gateway | 9311 | HTTP 健康检查端口 |
| coordinator | 9320 | 内部 HTTP 服务端口 |
| map | 9400 | 内部 HTTP 服务端口 |
| redis | 6379 | Redis 服务端口 |

gateway 的 Kubernetes Service 必须暴露 NodePort：

```text
containerPort: 9310
nodePort: 30910
```

## 8. 关键环境变量

gateway 至少需要：

```text
LAB4_GATEWAY_ADDR=0.0.0.0:9310
LAB4_GATEWAY_HEALTH_ADDR=0.0.0.0:9311
LAB4_COORDINATOR_URL=http://lab4-coordinator:9320
LAB4_NAMESPACE=lab4
```

coordinator 至少需要：

```text
LAB4_COORDINATOR_ADDR=:9320
LAB4_STATE_BACKEND=redis
LAB4_REDIS_ADDR=lab4-redis:6379
LAB4_REDIS_PREFIX=lab4
LAB4_GREEN_URL=http://lab4-map-green:9400
LAB4_CAVE_URL=http://lab4-map-cave:9400
LAB4_RUINS_URL=http://lab4-map-ruins:9400
LAB4_GREEN_NODE_ID=map-green
LAB4_CAVE_NODE_ID=map-cave
LAB4_RUINS_NODE_ID=map-ruins
LAB4_NAMESPACE=lab4
```

map-green 建议配置：

```text
LAB4_MAP_LISTEN_ADDR=:9400
LAB4_STATE_BACKEND=redis
LAB4_REDIS_ADDR=lab4-redis:6379
LAB4_REDIS_PREFIX=lab4
LAB4_MAP_ID=green
LAB4_NODE_ID=map-green
LAB4_COMPONENT=map-green
LAB4_NAMESPACE=lab4
```

map-cave 建议配置：

```text
LAB4_MAP_LISTEN_ADDR=:9400
LAB4_STATE_BACKEND=redis
LAB4_REDIS_ADDR=lab4-redis:6379
LAB4_REDIS_PREFIX=lab4
LAB4_MAP_ID=cave
LAB4_NODE_ID=map-cave
LAB4_COMPONENT=map-cave
LAB4_NAMESPACE=lab4
```

map-ruins 建议配置：

```text
LAB4_MAP_LISTEN_ADDR=:9400
LAB4_STATE_BACKEND=redis
LAB4_REDIS_ADDR=lab4-redis:6379
LAB4_REDIS_PREFIX=lab4
LAB4_MAP_ID=ruins
LAB4_NODE_ID=map-ruins
LAB4_COMPONENT=map-ruins
LAB4_NAMESPACE=lab4
```

## 9. HPA 和 Pod 回收要求

5 个业务 Deployment 都应该配置 HPA：

```text
lab4-gateway
lab4-coordinator
lab4-map-green
lab4-map-cave
lab4-map-ruins
```

HPA 要求：

```text
minReplicas: 1
maxReplicas: 10
```

CPU 目标值可以自己设置。建议先从 50% 到 70% 之间尝试，再根据 scoretest 结果调优。

Redis 不建议配置 HPA。Redis 是本实验中的状态中心，随意扩缩容会带来数据一致性问题。测试脚本也会检查 Redis 不应配置 HPA。

HPA 缩容、滚动更新、手动下线 Pod 时，Kubernetes 可能会回收正在承载玩家的 Pod。为了尽量做到用户无感，你需要设计生命周期处理。

本代码中已经提供了一些支持逻辑，主要思路是：

```text
1. Pod 准备接收流量时，readinessProbe 返回成功。
2. Pod 准备退出时，preStop 调用服务的 drain 接口。
3. 服务进入 draining 状态后，readinessProbe 返回失败。
4. Service 不再把新请求转发到该 Pod。
5. Pod 尽量保存状态，等待已有操作结束后退出。
```

如果你希望组件能主动调整 Pod deletion-cost，业务 Pod 需要访问 Kubernetes API patch 自己的 annotations。通常需要配置：

```text
ServiceAccount
Role
RoleBinding
```

Role 至少需要允许业务 Pod 对本命名空间内的 Pod 做必要的 `get`、`list`、`patch` 操作。具体权限请根据你的实现设计，原则是够用即可，不要给过大的集群权限。

## 10. 评分说明

基础测试只检查是否部署成功、功能是否完整。

竞技评分满分 100 分：

```text
空闲不误扩：10
低压稳定性：15
高压弹性扩容：25
状态一致性：20
异常恢复：20
资源纪律：10
```

评分脚本不会简单按绝对 TPS 或外部访问延迟打分，避免把学生电脑性能、服务器硬件、网络质量等不可控因素算进成绩。它更关注：

- 该扩容时是否扩容。
- 不该扩容时是否稳定。
- 扩容后服务是否仍然正确。
- Pod 被下线或回收时，状态是否能恢复。
- 资源 requests/limits、HPA 和探针配置是否合理。

## 11. 常见问题

`scoretest` 看起来卡住了：

```text
正常情况下脚本会每隔几秒输出阶段日志。
如果长时间没有日志，请检查 kubectl 是否能访问集群、Gateway 地址是否可连通。
```

HPA 一直是 `<unknown>`：

```text
metrics-server 不可用，或者 Deployment 没有配置 CPU requests。
```

Pod 是 `ImagePullBackOff`：

```text
节点拉不到镜像。请检查镜像名、imagePullPolicy、镜像仓库权限，或确认镜像已导入到对应节点。
```

Pod 是 `CrashLoopBackOff`：

```text
通常是启动命令、环境变量、端口、Redis 地址或 Service DNS 配错。
先看 kubectl -n lab4 logs <pod-name>。
```

Service 能创建但外部访问不到：

```text
检查 lab4-gateway 是否是 NodePort。
检查 nodePort 是否是 30910。
检查云平台网络规则是否允许访问 30910。
检查你访问的是可从本机访问的节点地址。
```

为什么明明 `docker images` 有镜像，Pod 还是拉不到：

```text
Kubernetes 使用的运行时可能是 containerd。
Pod 能否启动取决于 Kubernetes 运行时能否看到镜像，不完全取决于 docker images。
优先用 ctr -n k8s.io images ls 或 crictl images 检查。
```

为什么我的 Pod 没有平均分布到所有节点：

```text
Kubernetes 调度器会综合资源、已有 Pod、亲和性、污点、镜像可用性等因素选择节点。
它不保证每个节点数量完全相同。
```

Pod 会自动迁移吗：

```text
不会像虚拟机一样原地迁移。
通常是旧 Pod 下线或扩缩容创建了新 Pod，新 Pod 被调度到其他节点。
```

## 12. 可以优化的方向

基础部署成功后，可以从下面方向提高竞技评分：

- 减小镜像体积，提高镜像导入和 Pod 启动速度。
- 合理设置 CPU requests，让 HPA 既不误扩，也能及时扩容。
- 给不同组件设置不同资源，避免所有组件一刀切。
- 优化 coordinator 和 map 的热点逻辑，减少无意义 CPU 消耗。
- 让 gateway、coordinator、map 在 Pod 回收时更平滑地进入 drain。
- 改进状态保存和恢复逻辑，降低异常期间的数据丢失概率。
- 调整 HPA behavior，让扩容更及时、缩容更稳。
- 设计更合理的 Pod 分布策略，避免所有副本堆在同一个节点。

本实验的重点不是“照着模板部署成功”，而是理解一个真实游戏服务上云后会遇到的弹性、状态、异常恢复和成本问题。部署只是起点，调优才是区分度所在。
