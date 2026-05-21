# Lab4：自建 Kubernetes 集群部署游戏实验手册

目标是：学生拿到 `Lab4` 后，可以把分布式游戏部署到自己的 Kubernetes 集群，并验证 HPA 自动扩缩容。

默认部署结果：

```text
命名空间：lab4
公网入口：<任意 Node 公网 IP>:30910
对外 Service：lab4-gateway NodePort 30910
HPA：gateway / coordinator / map-green / map-cave / map-ruins 全部开启
```

## 举例快速部署流程（后面机器ip都用这个举例）

例如你的集群拓扑：

```text
k8s-a 公网 120.79.8.174，内网 10.0.1.10，control-plane
k8s-b 内网 10.0.2.10，worker
k8s-c 内网 10.0.2.11，worker
k8s-d 内网 10.0.2.12，worker
```

推荐在 `k8s-a` 上构建、导入镜像和部署，因为 worker 只有内网 IP，本地电脑通常不能直接访问 `10.0.2.x`。

位置：`[本地电脑]`

```bash
scp -r Lab4 root@120.79.8.174:/root/Lab4
ssh root@120.79.8.174
```

位置：`[K8s 主节点 k8s-a]`

```bash
cd /root/Lab4
./scripts/build-images.sh local
./scripts/import-images-to-nodes.sh \
.dist/images/lab4-images-local.tar \
10.0.2.10 10.0.2.11 10.0.2.12
./scripts/deploy-k8s.sh \
lab4/gateway:local \
lab4/coordinator:local \
lab4/map:local
kubectl -n lab4 get pods -o wide
kubectl -n lab4 get hpa
kubectl -n lab4 get svc lab4-gateway
go run ./cmd/admin 状态 120.79.8.174:30910
```

说明：当前 `k8s-a` 是 control-plane，默认有 `NoSchedule` 污点，业务 Pod 主要会跑在 `k8s-b/c/d`，所以镜像必须导入这三台 worker。若以后允许 `k8s-a` 也跑业务 Pod，再在 `k8s-a` 本机执行 `ctr -n k8s.io images import .dist/images/lab4-images-local.tar`。

## 0. 先分清楚命令在哪里执行

这份实验最容易混乱的地方是：有些命令在学生自己的电脑上执行，有些命令在 Kubernetes 主节点上执行，有些命令要在每台 worker 节点上执行。

本文用下面的标记区分：

```text
[本地电脑] 学生自己的电脑，Windows/macOS/Linux 都可以
[K8s 主节点] control-plane 节点，通常能运行 kubectl
[每台 worker] 所有可能运行业务 Pod 的 worker 节点
[任意 kubectl] 只要这个环境能 kubectl 连上集群即可，本地电脑或主节点都行
```

推荐路线：

```text
会用镜像仓库：本地电脑构建镜像 -> 推到仓库 -> kubectl 部署
不用镜像仓库：本地电脑构建镜像 tar -> 上传并导入每台 worker -> kubectl 部署
```

如果学生是 Windows，有两种方式：

```text
推荐：Windows + WSL2 Ubuntu，在 WSL 里按 Linux 命令执行
可选：Windows PowerShell，使用 scripts/*.ps1 脚本
```

## 1. 目录说明

```text
Lab4/
cmd/
admin/ # 管理命令，用于查看游戏服务状态
client/ # 交互式游戏客户端
gateway-loadtest/ # HPA 压测客户端
cloud-gateway/ # 云网关入口，接收 TCP 游戏协议
cloud-coordinator/ # 协调器，管理登录、会话、跨地图操作
cloud-map/ # 地图服务，承载 green/cave/ruins 地图
cloud/ # 游戏协调器与地图服务核心逻辑
cloudapi/ # gateway 与 coordinator/map 之间的 HTTP API
cluster/ # Kubernetes ConfigMap 状态与 leader 选举
protocol/ # 客户端 TCP 协议
storage/ # 用户与会话状态存储
twopc/ # 2PC 事务协调逻辑
world/ # 游戏世界、地图、NPC、道具逻辑
deploy/k8s/ # Kubernetes 部署清单
scripts/
build-images.sh # Linux/macOS/WSL 构建镜像
build-images.ps1 # Windows PowerShell 构建镜像
import-images-to-nodes.sh # Linux/macOS/WSL 导入镜像到节点
import-images-to-nodes.ps1# Windows PowerShell 导入镜像到节点
deploy-k8s.sh # Linux/macOS/WSL 部署到 K8s
deploy-k8s.ps1 # Windows PowerShell 部署到 K8s
load-test-hpa.sh # Linux/macOS/WSL 压测 HPA
load-test-hpa.ps1 # Windows PowerShell 压测 HPA
Dockerfile.cloud-* # 三类服务镜像 Dockerfile
go.mod
```

这份包不包含 `.gocache`、`.dist`、旧部署目录、本地临时文件。

## 2. 部署架构

部署后会启动 5 个 Deployment：

```text
lab4-gateway 对外入口，NodePort 30910
lab4-coordinator 协调登录、会话、跨地图、2PC
lab4-map-green green 地图
lab4-map-cave cave 地图
lab4-map-ruins ruins 地图
```

访问链路：

```text
客户端
-> <任意 Node IP>:30910
-> lab4-gateway Pod
-> lab4-coordinator Service
-> lab4-map-* Service
-> 原路返回客户端
```

状态与一致性：

```text
Kubernetes ConfigMap / etcd
-> 保存用户、会话、地图 checkpoint
-> etcd 底层使用 Raft 保证 Kubernetes 元数据一致性
leader ConfigMap
-> coordinator 和每个 map 组件都会选一个 leader
-> HPA 扩出多个副本时，非 leader 副本会把业务请求代理给 leader
-> 避免多个副本同时修改同一份游戏状态导致状态分裂
```

注意：这是实验版设计，适合课程验证 HPA、leader、2PC、ConfigMap/etcd 一致性语义。生产级高频游戏状态更适合 Redis/MySQL/专用状态服务。

## 3. 前置条件

### 3.1 Kubernetes 集群要求

学生需要先有一个可以工作的 Kubernetes 集群。推荐：

```text
1 台 control-plane 主节点
2-3 台 worker 节点
所有节点内网互通
默认只有 control-plane 主节点有公网 IP
worker 节点默认只有内网 IP
云服务器安全组放行 TCP 6443 和 TCP 30910
```

本文默认采用下面这种云服务器拓扑：

```text
学生本机
-> 通过主节点公网 IP 访问 Kubernetes API Server:6443
-> 通过主节点公网 IP 访问游戏 NodePort:30910
-> 不能直接访问 worker 的 10.x / 172.x / 192.168.x 内网 IP
control-plane 主节点
-> 有公网 IP 和内网 IP
-> 能通过内网访问所有 worker
worker 节点
-> 只有内网 IP
-> 运行游戏 Pod
```

也就是说：本地电脑只需要能访问主节点公网 IP；worker 是内网 IP 没关系，镜像导入和 Pod 调度都可以通过主节点完成。

每台机器建议：

```text
2 核 4GB 起步
推荐 4 核 8GB 或更高
Ubuntu 22.04 / Debian / CentOS 均可
```

集群需要已经安装：

```text
containerd 或 Docker
kubelet
kubeadm
kubectl
网络插件，例如 flannel / calico
metrics-server
```

### 3.2 本地电脑要求

如果在本地电脑构建镜像，需要安装：

```text
Go 1.21 或更高
Docker Desktop 或 Docker Engine
kubectl
OpenSSH 客户端
```

Windows 学生推荐安装：

```text
WSL2 Ubuntu
Docker Desktop，并开启 WSL integration
Windows Terminal
OpenSSH Client
kubectl
Go
```

Windows 如果不用 WSL，也可以用 PowerShell。本文所有 `.ps1` 脚本都支持 PowerShell。

如果 PowerShell 不允许执行脚本，可以临时放开当前窗口权限：

```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
```

## 4. 拿到文件后怎么进入目录

### 4.1 Linux/macOS/WSL

位置：`[本地电脑]`

```bash
cd Lab4
```

### 4.2 Windows PowerShell

位置：`[本地电脑]`

```powershell
cd Lab4
```

## 5. 检查集群是否正常

位置：`[任意 kubectl]`

只要当前环境能连接到学生自己的集群即可。可以在主节点上执行，也可以在本地电脑配置好 [kubeconfig（5.1）](#51-在本机配置-kubeconfig-直连自己的集群) 后执行。

```bash
kubectl get nodes -o wide
kubectl get pods -A
```

所有节点应为 `Ready`。

检查 metrics-server：

```bash
kubectl top nodes
kubectl top pods -A
```

如果 `kubectl top` 报错，HPA 会显示 `<unknown>`，需要先安装或修复 [metrics-server（5.2）](#52-安装-metrics-server)。

### 5.1 在本机配置 kubeconfig 直连自己的集群

本节目标：让学生在自己的电脑上直接执行 `kubectl` 控制自己的 K8s 集群，而不是每次都 SSH 到主节点。

默认前提：

```text
主节点有公网 IP，例如 <主节点公网IP>
worker 只有内网 IP，本地电脑不能直连 worker
本地电脑只需要连主节点的 6443 端口
```

位置：`[云厂商控制台]`

先在主节点安全组放行 Kubernetes API Server 端口：

```text
TCP 6443
```

建议只放行老师/学生自己电脑的公网出口 IP，不要长期对 `0.0.0.0/0` 完全开放。

位置：`[K8s 主节点]`

确认 apiserver 证书里包含主节点公网 IP：

```bash
openssl x509 -in /etc/kubernetes/pki/apiserver.crt -noout -text \
| grep -A2 "Subject Alternative Name"
```

输出里应该能看到类似：

```text
IP Address:<主节点公网IP>
```

如果证书里没有主节点公网 IP，本机直接访问 `https://<主节点公网IP>:6443` 时可能会报证书错误。推荐在初始化集群时使用：

```bash
kubeadm init --apiserver-cert-extra-sans=<主节点公网IP>
```

如果集群已经建好但证书没有公网 IP，需要重新签发 apiserver 证书或重新初始化集群；不建议为了省事把 kubeconfig 改成跳过 TLS 校验。

#### Linux / macOS / WSL

位置：`[本地电脑]`

把主节点上的管理员 kubeconfig 拷到本机：

如果本机已经有别的集群配置，并且想保留它，先看本节最后的“如果本机已经有别的 kubeconfig”。

```bash
mkdir -p ~/.kube
scp root@<主节点公网IP>:/etc/kubernetes/admin.conf ~/.kube/config
chmod 600 ~/.kube/config
```

把 kubeconfig 里的 apiserver 地址从内网 IP 改成主节点公网 IP：

```bash
kubectl config set-cluster kubernetes \
--server=https://<主节点公网IP>:6443
```

给 context 起一个清楚的名字，并默认进入 `lab4` 命名空间：

```bash
kubectl config rename-context kubernetes-admin@kubernetes lab4-cluster
kubectl config set-context lab4-cluster --namespace=lab4
kubectl config use-context lab4-cluster
```

验证本机已经能控制集群：

```bash
kubectl get nodes -o wide
kubectl get pods,hpa -o wide
```

因为默认 namespace 已经设成 `lab4`，后面查看实验资源时通常不用再加 `-n lab4`。

#### Windows PowerShell

位置：`[本地电脑]`

Windows 也可以直接配置 kubeconfig：

如果本机已经有别的集群配置，并且想保留它，先看本节最后的“如果本机已经有别的 kubeconfig”。

```powershell
New-Item -ItemType Directory -Force $env:USERPROFILE\.kube
scp root@<主节点公网IP>:/etc/kubernetes/admin.conf $env:USERPROFILE\.kube\config
```

把 apiserver 地址改成主节点公网 IP：

```powershell
kubectl config set-cluster kubernetes --server=https://<主节点公网IP>:6443
```

设置 context 和默认命名空间：

```powershell
kubectl config rename-context kubernetes-admin@kubernetes lab4-cluster
kubectl config set-context lab4-cluster --namespace=lab4
kubectl config use-context lab4-cluster
```

验证：

```powershell
kubectl get nodes -o wide
kubectl get pods,hpa -o wide
```

#### 如果本机已经有别的 kubeconfig

如果本机之前配置过别的集群，覆盖前可以先备份：

Linux / macOS / WSL：

```bash
[ -f ~/.kube/config ] && cp ~/.kube/config ~/.kube/config.bak.$(date +%Y%m%d%H%M%S)
```

Windows PowerShell：

```powershell
if (Test-Path $env:USERPROFILE\.kube\config) {
Copy-Item $env:USERPROFILE\.kube\config "$env:USERPROFILE\.kube\config.bak"
}
```

初学实验最简单的做法是直接使用本实验集群的 `admin.conf` 作为 `~/.kube/config`。如果需要同时管理多个集群，可以之后再学习 `KUBECONFIG` 合并多个配置文件。

### 5.2 安装 metrics-server

如果集群已经安装 metrics-server，可以跳过本节。

位置：`[任意 kubectl]`

安装官方 v0.8.1 清单：

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.8.1/components.yaml
```

国内云主机默认建议把镜像改成阿里云镜像仓库，避免 `registry.k8s.io` 拉取不稳定：

```bash
kubectl -n kube-system set image deployment/metrics-server \
metrics-server=registry.aliyuncs.com/google_containers/metrics-server:v0.8.1
```

这份镜像的 tag 是 `v0.8.1`，本实验已用 `docker manifest inspect registry.aliyuncs.com/google_containers/metrics-server:v0.8.1` 确认可以解析到 amd64 镜像。

很多自建 kubeadm 集群的 kubelet 证书不是 metrics-server 默认信任的证书，需要加：

```text
--kubelet-insecure-tls
```

执行：

```bash
kubectl -n kube-system patch deployment metrics-server --type=json \
-p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
```

等待启动：

```bash
kubectl -n kube-system rollout status deployment/metrics-server --timeout=180s
kubectl top nodes
```

Windows PowerShell 也可以直接执行上面命令。如果 JSON 引号在 PowerShell 里出问题，用下面这个：

```powershell
kubectl -n kube-system patch deployment metrics-server --type=json -p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
```

## 6. 选择镜像分发方式

镜像分发有两种方式，二选一即可。

```text
方式 A：使用镜像仓库
优点：最标准，部署最简单
缺点：需要 Docker Hub/TCR/ACR/Harbor 等仓库
方式 B：不用镜像仓库，导入镜像 tar 到每台节点
优点：不需要外部仓库
缺点：每台可能运行 Pod 的节点都要导入一次镜像
```

初学者如果没有镜像仓库，就用方式 B。

## 7. 方式 A：使用镜像仓库

### 7.1 Linux/macOS/WSL 构建并推送

位置：`[本地电脑]`

假设镜像仓库前缀是：

```text
docker.io/yourname/lab4
```

执行：

```bash
cd Lab4
go test ./...
IMAGE_PREFIX=docker.io/yourname/lab4 PUSH=1 ./scripts/build-images.sh v1
```

会构建并推送：

```text
docker.io/yourname/lab4/gateway:v1
docker.io/yourname/lab4/coordinator:v1
docker.io/yourname/lab4/map:v1
```

### 7.2 Windows PowerShell 构建并推送

位置：`[本地电脑]`

```powershell
cd Lab4
go test ./...
.\scripts\build-images.ps1 -Tag v1 -ImagePrefix docker.io/yourname/lab4 -Push
```

### 7.3 使用仓库镜像部署

位置：`[任意 kubectl]`

Linux/macOS/WSL：

```bash
./scripts/deploy-k8s.sh \
docker.io/yourname/lab4/gateway:v1 \
docker.io/yourname/lab4/coordinator:v1 \
docker.io/yourname/lab4/map:v1
```

Windows PowerShell：

```powershell
.\scripts\deploy-k8s.ps1 `
-GatewayImage docker.io/yourname/lab4/gateway:v1 `
-CoordinatorImage docker.io/yourname/lab4/coordinator:v1 `
-MapImage docker.io/yourname/lab4/map:v1
```

### 7.4 私有仓库需要 imagePullSecret

位置：`[任意 kubectl]`

如果镜像仓库是私有仓库，先创建命名空间和 Secret：

```bash
kubectl create namespace lab4 --dry-run=client -o yaml | kubectl apply -f -
kubectl -n lab4 create secret docker-registry image-pull-secret \
--docker-server=<仓库地址> \
--docker-username=<用户名> \
--docker-password=<密码>
```

给运行服务的 ServiceAccount 配置拉取密钥：

```bash
kubectl -n lab4 patch serviceaccount lab4-runner \
-p '{"imagePullSecrets":[{"name":"image-pull-secret"}]}'
```

PowerShell 版本：

```powershell
kubectl create namespace lab4 --dry-run=client -o yaml | kubectl apply -f -
kubectl -n lab4 create secret docker-registry image-pull-secret `
--docker-server=<仓库地址> `
--docker-username=<用户名> `
--docker-password=<密码>
kubectl -n lab4 patch serviceaccount lab4-runner -p '{"imagePullSecrets":[{"name":"image-pull-secret"}]}'
```

## 8. 方式 B：不用镜像仓库，导入每台节点

这个方式最适合没有镜像仓库的实验环境。本文默认 worker 只有内网 IP，本地电脑不能直连 worker，所以最推荐先把 `Lab4` 上传到主节点，在主节点上完成构建、导入和部署。

核心原则：

```text
只要某台节点可能运行 lab4 的 Pod，那台节点就必须导入镜像 tar。
```

一般情况下，control-plane 主节点有 `NoSchedule` taint，普通业务 Pod 只会跑到 worker 节点，所以导入所有 worker 即可。

如果你去掉了主节点 taint，或者允许业务 Pod 调度到主节点，也要把镜像导入主节点。

### 8.1 推荐方式：直接在主节点上构建和导入

如果学生本地电脑是 Windows，或者本地无法稳定使用 Docker，也可以把 `Lab4` 目录传到主节点，在主节点执行构建和导入。这个方式对“只有主节点有公网 IP、worker 只有内网 IP”的云服务器集群最省心。

位置：`[本地电脑]`

```bash
scp -r Lab4 root@<主节点公网IP>:/root/Lab4
```

Windows PowerShell：

```powershell
scp -r .\Lab4 root@<主节点公网IP>:/root/Lab4
```

位置：`[K8s 主节点]`

```bash
ssh root@<主节点公网IP>
cd /root/Lab4
go test ./...
./scripts/build-images.sh v1
```

主节点可以直接访问 worker 内网 IP，不需要 `JUMP_HOST`：

```bash
./scripts/import-images-to-nodes.sh \
.dist/images/lab4-images-v1.tar \
10.0.2.10 10.0.2.11 10.0.2.12
```

最后仍然在主节点部署：

```bash
./scripts/deploy-k8s.sh lab4/gateway:v1 lab4/coordinator:v1 lab4/map:v1
```

### 8.2 先确认哪些节点可能运行业务 Pod

位置：`[任意 kubectl]`

查看节点：

```bash
kubectl get nodes -o wide
```

示例：

```text
NAME STATUS ROLES INTERNAL-IP
k8s-a Ready control-plane 10.0.1.10
k8s-b Ready <none> 10.0.2.10
k8s-c Ready <none> 10.0.2.11
k8s-d Ready <none> 10.0.2.12
```

检查主节点是否允许调度业务 Pod：

```bash
kubectl describe node k8s-a | grep -i Taints
```

如果看到：

```text
node-role.kubernetes.io/control-plane:NoSchedule
```

说明普通业务 Pod 不会跑到主节点，只导入 worker 即可。

上面例子中需要导入：

```text
10.0.2.10
10.0.2.11
10.0.2.12
```

### 8.3 重要：worker 只有内网 IP 时怎么办

很多云服务器集群只有主节点有公网 IP，worker 只有内网 IP，例如：

```text
主节点公网 IP：120.79.8.174
主节点内网 IP：10.0.1.10
worker 内网 IP：10.0.2.10 / 10.0.2.11 / 10.0.2.12
```

这时学生的本地电脑不能直接访问 `10.0.2.x`，因为这些是云 VPC 内网地址。

有两种解决方式：

```text
首选：使用 8.1，把 Lab4 上传到主节点，在主节点构建镜像并导入 worker
备选：本地电脑执行导入脚本，但通过主节点公网 IP 做跳板机
```

本文后面的自动导入脚本支持跳板机。Linux/macOS/WSL 使用环境变量：

```bash
JUMP_HOST=<主节点公网IP>
```

Windows PowerShell 使用参数：

```powershell
-JumpHost <主节点公网IP>
```

如果主节点和 worker 的 SSH 用户都是 `root`，只需要指定 `JUMP_HOST`。如果跳板机用户不同，再指定 `JUMP_USER` 或 `-JumpUser`。

### 8.4 Linux/macOS/WSL 构建镜像 tar

位置：`[本地电脑]`

```bash
cd Lab4
go test ./...
./scripts/build-images.sh v1
```

构建完成后会得到：

```text
.dist/images/lab4-images-v1.tar
```

### 8.5 Windows PowerShell 构建镜像 tar

位置：`[本地电脑]`

```powershell
cd Lab4
go test ./...
.\scripts\build-images.ps1 -Tag v1
```

构建完成后会得到：

```text
.dist\images\lab4-images-v1.tar
```

### 8.6 自动导入到所有 worker：Linux/macOS/WSL

位置：`[本地电脑]`

如果 worker IP 是：

```text
10.0.2.10
10.0.2.11
10.0.2.12
```

worker 只有内网 IP，本地不能直连，例如主节点公网 IP 是 `120.79.8.174`，

如果节点使用 containerd，执行：

```bash
JUMP_HOST=120.79.8.174 ./scripts/import-images-to-nodes.sh \
.dist/images/lab4-images-v1.tar \
10.0.2.10 10.0.2.11 10.0.2.12
```

这个脚本会对每台节点执行：

```text
1. ssh root@<worker-ip> mkdir -p /root
2. scp lab4-images-v1.tar root@<worker-ip>:/root/
3. ssh root@<worker-ip> ctr -n k8s.io images import /root/lab4-images-v1.tar
4. ssh root@<worker-ip> crictl images | grep lab4
```

如果指定了 `JUMP_HOST`，上面所有 SSH/SCP 都会自动加跳板机，相当于：

```bash
ssh -J root@120.79.8.174 root@10.0.2.10
scp -o ProxyJump=root@120.79.8.174 lab4-images-v1.tar root@10.0.2.10:/root/
```

如果节点使用 Docker 而不是 containerd：

```bash
RUNTIME=docker ./scripts/import-images-to-nodes.sh \
.dist/images/lab4-images-v1.tar \
10.0.2.10 10.0.2.11 10.0.2.12
```

如果不确定节点运行时：

```bash
RUNTIME=auto ./scripts/import-images-to-nodes.sh \
.dist/images/lab4-images-v1.tar \
10.0.2.10 10.0.2.11 10.0.2.12
```

如果 SSH 用户不是 root，例如 `ubuntu`：

```bash
REMOTE_USER=ubuntu ./scripts/import-images-to-nodes.sh \
.dist/images/lab4-images-v1.tar \
10.0.2.10 10.0.2.11 10.0.2.12
```

注意：如果用非 root 用户，远端执行 `ctr` 或 `docker` 可能需要 sudo。初学实验建议直接用 root 用户，或者自行把脚本里的远端命令改成 sudo。

如果跳板机用户名和 worker 用户不同，例如跳板机是 `ubuntu`、worker 是 `root`：

```bash
REMOTE_USER=root JUMP_USER=ubuntu JUMP_HOST=120.79.8.174 ./scripts/import-images-to-nodes.sh \
.dist/images/lab4-images-v1.tar \
10.0.2.10 10.0.2.11 10.0.2.12
```

### 8.7 自动导入到所有 worker：Windows PowerShell

位置：`[本地电脑]`

如果本地电脑能直接访问 worker 内网 IP，PowerShell 执行：

```powershell
.\scripts\import-images-to-nodes.ps1 `
-ImageTar .dist\images\lab4-images-v1.tar `
-Nodes 10.0.2.10,10.0.2.11,10.0.2.12
```

如果 worker 只有内网 IP，本地不能直连，主节点公网 IP 是 `120.79.8.174`，执行：

```powershell
.\scripts\import-images-to-nodes.ps1 `
-ImageTar .dist\images\lab4-images-v1.tar `
-Nodes 10.0.2.10,10.0.2.11,10.0.2.12 `
-JumpHost 120.79.8.174
```

如果节点使用 Docker：

```powershell
.\scripts\import-images-to-nodes.ps1 `
-ImageTar .dist\images\lab4-images-v1.tar `
-Nodes 10.0.2.10,10.0.2.11,10.0.2.12 `
-Runtime docker
```

如果不确定节点运行时：

```powershell
.\scripts\import-images-to-nodes.ps1 `
-ImageTar .dist\images\lab4-images-v1.tar `
-Nodes 10.0.2.10,10.0.2.11,10.0.2.12 `
-Runtime auto
```

如果 SSH 用户不是 root：

```powershell
.\scripts\import-images-to-nodes.ps1 `
-ImageTar .dist\images\lab4-images-v1.tar `
-Nodes 10.0.2.10,10.0.2.11,10.0.2.12 `
-RemoteUser ubuntu
```

如果跳板机用户名和 worker 用户不同，例如跳板机是 `ubuntu`、worker 是 `root`：

```powershell
.\scripts\import-images-to-nodes.ps1 `
-ImageTar .dist\images\lab4-images-v1.tar `
-Nodes 10.0.2.10,10.0.2.11,10.0.2.12 `
-RemoteUser root `
-JumpUser ubuntu `
-JumpHost 120.79.8.174
```

Windows 的 `ssh` 和 `scp` 如果提示第一次连接确认，输入 `yes`。如果没有配置 SSH key，会提示输入节点密码。

### 8.8 手动导入到每台 worker

如果自动脚本失败，可以手动做。下面以 worker `10.0.2.10` 为例。

位置：`[本地电脑]`

worker 是内网 IP，需要通过主节点跳板：

```bash
scp -o ProxyJump=root@120.79.8.174 .dist/images/lab4-images-v1.tar root@10.0.2.10:/root/
```

位置：`[每台 worker]`

worker 是内网 IP，需要通过主节点跳板：

```bash
ssh -J root@120.79.8.174 root@10.0.2.10
```

如果节点是 containerd：

```bash
ctr -n k8s.io images import /root/lab4-images-v1.tar
crictl images | grep lab4
```

如果节点是 Docker：

```bash
docker load -i /root/lab4-images-v1.tar
docker images | grep lab4
```

对每个 worker 都重复一次。所有 worker 都应能看到：

```text
lab4/gateway:v1
lab4/coordinator:v1
lab4/map:v1
```

### 8.9 导入完成后部署

位置：`[任意 kubectl]`

Linux/macOS/WSL：

```bash
./scripts/deploy-k8s.sh \
lab4/gateway:v1 \
lab4/coordinator:v1 \
lab4/map:v1
```

Windows PowerShell：

```powershell
.\scripts\deploy-k8s.ps1 `
-GatewayImage lab4/gateway:v1 `
-CoordinatorImage lab4/coordinator:v1 `
-MapImage lab4/map:v1
```

清单里的 `imagePullPolicy` 是 `IfNotPresent`，所以节点本地有镜像时不会强制从外部仓库拉取。

## 9. 部署后检查

位置：`[任意 kubectl]`

查看 Pod：

```bash
kubectl -n lab4 get pods -o wide
```

正常应该看到 5 类 Pod：

```text
lab4-gateway
lab4-coordinator
lab4-map-green
lab4-map-cave
lab4-map-ruins
```

查看 Service：

```bash
kubectl -n lab4 get svc
```

应看到：

```text
lab4-gateway NodePort ... 9310:30910/TCP
```

查看 HPA：

```bash
kubectl -n lab4 get hpa
```

应看到 5 个 HPA：

```text
lab4-gateway
lab4-coordinator
lab4-map-green
lab4-map-cave
lab4-map-ruins
```

查看 leader 与 checkpoint：

```bash
kubectl -n lab4 get configmap
```

运行一会儿后应看到：

```text
lab4-leader-coordinator
lab4-leader-map-green
lab4-leader-map-cave
lab4-leader-map-ruins
lab4-map-green-checkpoint
lab4-map-cave-checkpoint
lab4-map-ruins-checkpoint
```

## 10. 开放公网访问

位置：`[云厂商控制台]`

在安全组里放行：

```text
TCP 30910
```

然后客户端访问：

```text
<任意 Node 公网 IP>:30910
```

即使 gateway Pod 运行在 worker，访问主节点公网 IP 也通常可以，因为 NodePort 会通过 kube-proxy 转发到实际 gateway Pod。

## 11. 验证游戏服务

位置：`[本地电脑]`

查看协调器状态：

Linux/macOS/WSL：

```bash
go run ./cmd/admin 状态 <公网IP>:30910
```

Windows PowerShell：

```powershell
go run .\cmd\admin 状态 <公网IP>:30910
```

预期输出类似：

```text
云上拆分版协调器状态：
- cave -> map-cave (http://lab4-map-cave:9400)
- green -> map-green (http://lab4-map-green:9400)
- ruins -> map-ruins (http://lab4-map-ruins:9400)
```

启动交互式客户端：

Linux/macOS/WSL：

```bash
go run ./cmd/client <公网IP>:30910
```

Windows PowerShell：

```powershell
go run .\cmd\client <公网IP>:30910
```

## 12. HPA 配置说明

HPA 清单在：

```text
deploy/k8s/hpa.yaml
```

每个组件都有 HPA：

```text
lab4-gateway
lab4-coordinator
lab4-map-green
lab4-map-cave
lab4-map-ruins
```

默认配置：

```yaml
minReplicas: 1
maxReplicas: 10
metrics:
- type: Resource
resource:
name: cpu
target:
type: Utilization
averageUtilization: 60
```

含义：

```text
最少 1 个副本
最多 10 个副本
按 CPU 平均利用率扩缩容
目标利用率 60%
```

这里的 `60%` 是相对于 Pod 的 `resources.requests.cpu` 计算，不是相对于整台机器 CPU 计算。

例如 gateway：

```yaml
resources:
requests:
cpu: 50m
memory: 64Mi
limits:
cpu: 300m
memory: 128Mi
```

`requests.cpu: 50m` 时，HPA 目标 60% 大约等价于每个 Pod 平均使用 `30m CPU`。

如果看到：

```text
cpu: <unknown>/60%
```

通常是 metrics-server 没准备好，或 Pod 刚启动，等待 30-60 秒再看。

## 13. HPA 压测

压测脚本使用真实游戏 TCP 协议，不是 HTTP 压测。它只负责打流量，不再混合输出 `kubectl get` 结果，这样压测日志会更干净。

观察 HPA 请另开一个终端，在任意能执行 `kubectl` 的节点或本机运行：

```bash
watch -n 0.5 'kubectl -n lab4 get pods,hpa -o wide'
```

如果是 Windows PowerShell 且没有 `watch`，可以用：

```powershell
while ($true) { Clear-Host; Get-Date; kubectl -n lab4 get pods,hpa -o wide; Start-Sleep -Milliseconds 500 }
```

然后在另一个终端运行下面的压测命令。压测期间重点看 `HPA TARGETS`、`REPLICAS` 和新增 Pod 是否进入 `Running`。

### 13.1 Linux/macOS/WSL 压测

位置：`[本地电脑]`

```bash
ADDR=<公网IP>:30910 ./scripts/load-test-hpa.sh
```

调大压力：

```bash
ADDR=<公网IP>:30910 CLIENTS=600 OPS_PER_CLIENT=8 DURATION=10m ./scripts/load-test-hpa.sh
```

### 13.2 Windows PowerShell 压测

位置：`[本地电脑]`

```powershell
.\scripts\load-test-hpa.ps1 -Addr <公网IP>:30910
```

调大压力：

```powershell
.\scripts\load-test-hpa.ps1 -Addr <公网IP>:30910 -Clients 600 -OpsPerClient 8 -Duration 10m
```

判断 HPA 生效：

```text
REPLICAS 从 1 增加到更多
TARGETS 中当前 CPU 接近或超过 60%
新增 Pod 进入 Running
```

压测结束后，缩容不会立刻发生。Kubernetes HPA 默认会有缩容稳定窗口，通常要等几分钟才会慢慢缩回去。

## 14. 常见问题

### 14.1 Windows 上不能运行 .sh

Windows 不直接运行 `.sh`。有三种选择：

```text
1. 推荐使用 WSL2 Ubuntu，然后使用 README 里的 Linux 命令
2. 使用 Git Bash 运行 .sh
3. 使用本文提供的 .ps1 PowerShell 脚本
```

PowerShell 第一次运行脚本可能被策略拦截：

```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
```

### 14.2 Docker Desktop 构建 linux/amd64 很慢

Windows/macOS 上 Docker Desktop 构建 linux/amd64 镜像可能会慢一点，这是正常的。

如果学生本地电脑构建很慢，可以把 `Lab4` 上传到 K8s 主节点，在主节点上执行 Linux 构建命令：

位置：`[K8s 主节点]`

```bash
cd Lab4
./scripts/build-images.sh v1
```

然后按方式 B，把镜像 tar 导入 worker。

### 14.3 Pod 一直 ImagePullBackOff

常见原因：

```text
镜像名写错
没有推到仓库
私有仓库没有 imagePullSecret
不用仓库时没有在每台节点导入镜像
```

检查：

```bash
kubectl -n lab4 describe pod <pod-name>
```

如果使用 tar 包方式，确认每台 worker 都有镜像：

```bash
crictl images | grep lab4
```

或者：

```bash
docker images | grep lab4
```

### 14.4 HPA 一直是 `<unknown>`

检查：

```bash
kubectl -n kube-system get pods | grep metrics-server
kubectl top nodes
kubectl top pods -n lab4
```

如果 `kubectl top` 不可用，先修 metrics-server。

### 14.5 访问公网 IP:30910 不通

检查 Service：

```bash
kubectl -n lab4 get svc lab4-gateway
```

确认有：

```text
9310:30910/TCP
```

检查安全组：

```text
云服务器安全组必须放行 TCP 30910
```

检查 Pod：

```bash
kubectl -n lab4 get pods -l app=lab4-gateway -o wide
```

### 14.6 为什么 Pod 不跑在主节点

如果主节点是 control-plane，通常有 taint：

```bash
kubectl describe node <主节点名> | grep -i Taints
```

常见输出：

```text
node-role.kubernetes.io/control-plane:NoSchedule
```

这表示普通业务 Pod 默认不会调度到主节点，是正常现象。

### 14.7 coordinator 扩到多个副本会不会状态乱

这份实验包里，coordinator 和 map 都有 ConfigMap leader 选举。HPA 扩出多个副本后，只有 leader 处理状态变更，非 leader 会把请求代理给 leader。

正常实验场景下不会因为多副本导致状态分裂。

但这不是生产级强状态存储方案。真实生产环境建议把用户、会话、地图状态迁移到 Redis/MySQL/专用状态服务。

## 15. 常用修改点

### 15.1 修改命名空间

默认：

```text
lab4
```

如果要改，需要同步修改：

```text
deploy/k8s/kustomization.yaml
deploy/k8s/namespace.yaml
deploy/k8s/*.yaml 里的 namespace
scripts/deploy-k8s.sh 里的 NAMESPACE 默认值
scripts/deploy-k8s.ps1 里的 Namespace 默认值
```

不建议初学者一开始改命名空间。

### 15.2 修改 NodePort

默认：

```text
30910
```

修改文件：

```text
deploy/k8s/gateway-service.yaml
```

字段：

```yaml
nodePort: 30910
```

NodePort 合法范围通常是：

```text
30000-32767
```

### 15.3 修改 HPA 副本范围

修改文件：

```text
deploy/k8s/hpa.yaml
```

字段：

```yaml
minReplicas: 1
maxReplicas: 10
averageUtilization: 60
```

### 15.4 修改 Pod 资源

修改各 Deployment：

```text
deploy/k8s/gateway-deployment.yaml
deploy/k8s/coordinator-deployment.yaml
deploy/k8s/map-green-deployment.yaml
deploy/k8s/map-cave-deployment.yaml
deploy/k8s/map-ruins-deployment.yaml
```

字段：

```yaml
resources:
requests:
cpu: 50m
memory: 64Mi
limits:
cpu: 300m
memory: 128Mi
```

HPA 的 CPU 百分比以 `requests.cpu` 为基准。

## 16. 清理实验

位置：`[任意 kubectl]`

删除整个实验：

```bash
kubectl delete namespace lab4
```

这会删除 Deployment、Service、HPA、ConfigMap、leader、checkpoint 和用户状态。

如果只是重新部署新镜像，不要删除 namespace，重新执行部署脚本即可：

Linux/macOS/WSL：

```bash
./scripts/deploy-k8s.sh <gateway-image> <coordinator-image> <map-image>
```

Windows PowerShell：

```powershell
.\scripts\deploy-k8s.ps1 -GatewayImage <gateway-image> -CoordinatorImage <coordinator-image> -MapImage <map-image>
```
