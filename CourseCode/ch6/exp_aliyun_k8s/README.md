# 阿里云 K8s 演示（基于 exp_aliyun_docker 三层服务）

本实验以三层服务（storage/game/gateway）为例，完成：

1) kubeadm 初始化控制平面与加入工作节点
2) 构建镜像并推送到本地镜像仓库
3) 部署到 K8s、验证访问
4) 反亲和性与拓扑分布、HPA/VPA 实验

> **重点提示**：以下步骤按“第一次搭建集群”的流程组织，不包含节点删除/重加入的内容。

## 目录结构

```text
exp_aliyun_k8s/
├── k8s/
│   ├── storage-deployment.yaml
│   ├── storage-service.yaml
│   ├── game-deployment.yaml
│   ├── game-service.yaml
│   ├── gateway-deployment.yaml
│   ├── gateway-service.yaml
│   ├── hpa.yaml
│   └── vpa.yaml
└── README.md
```

## 0. 使用 kubeadm 初始化集群（第一次搭建）

### 0.1 控制平面初始化（k8s-a）

这里仅展示最小化的配置，实际生产环境需要更多安全/网络配置。
```bash
kubeadm init \
  --apiserver-advertise-address=<MASTER_IP> \
  --pod-network-cidr=10.244.0.0/16
```

配置 kubectl：

```bash
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
```

安装 CNI（示例 flannel）：

```bash
kubectl apply -f https://raw.githubusercontent.com/flannel-io/flannel/master/Documentation/kube-flannel.yml
```

**预期结果 ✅**

- `kubectl get nodes` 能看到控制平面节点，状态为 `Ready`

### 0.2 工作节点加入集群（k8s-b/k8s-c/k8s-d）

在控制平面生成 join 命令：

```bash
kubeadm token create --print-join-command
```

在每个工作节点执行输出的 join 命令：

```bash
kubeadm join <MASTER_IP>:6443 --token <TOKEN> --discovery-token-ca-cert-hash sha256:<HASH>
```

**预期结果 ✅**

- `kubectl get nodes` 可看到 4 个节点全部 `Ready`

## 1. 构建并推送镜像到本地 registry

假设：

- 本地镜像仓库：`10.0.2.12:5000/exp_aliyun_k8s`

```bash
export ACR=10.0.2.12:5000/exp_aliyun_k8s

docker build -f CourseCode/ch6/exp_aliyun_docker/docker/storage.Dockerfile -t ${ACR}/game-storage:v1 CourseCode/ch6/exp_aliyun_docker
docker build -f CourseCode/ch6/exp_aliyun_docker/docker/game.Dockerfile -t ${ACR}/game-service:v1 CourseCode/ch6/exp_aliyun_docker
docker build -f CourseCode/ch6/exp_aliyun_docker/docker/gateway.Dockerfile -t ${ACR}/game-gateway:v1 CourseCode/ch6/exp_aliyun_docker

docker push ${ACR}/game-storage:v1
docker push ${ACR}/game-service:v1
docker push ${ACR}/game-gateway:v1
```

**预期结果 ✅**

- registry 中出现 `game-storage/game-service/game-gateway` 镜像

## 2. 让节点能够拉取私有镜像（HTTP/insecure）

如果 registry 使用 HTTP（非 TLS），需要在每个节点配置：

```json
{
  "insecure-registries": ["10.0.2.12:5000"]
}
```

```bash
sudo systemctl restart docker
```

> 若集群使用 containerd，请在 `/etc/containerd/config.toml` 中添加 mirror 配置并重启 containerd。

## 3. 部署到 Kubernetes

```bash
kubectl create namespace exp-aliyun-k8s 2>/dev/null || true
kubectl -n exp-aliyun-k8s apply -f k8s/
kubectl -n exp-aliyun-k8s get pods,svc
```

**预期结果 ✅**

- `storage/game/gateway` Pod 均为 `Running`
- `gateway` Service 出现 NodePort（30080）

## 4. 访问服务（演示）

- NodePort：`http://<NODE_IP>:30080`
- 或 port-forward：

```bash
kubectl -n exp-aliyun-k8s port-forward svc/gateway 18080:8080
```

**预期结果 ✅**

- 能从浏览器或 curl 访问到网关接口

## 5. Pod 反亲和性 + 拓扑分布约束实验

`k8s/game-deployment.yaml` 已加入：

- `podAntiAffinity`：尽量将 game Pod 分散到不同节点
- `topologySpreadConstraints`：限制同一节点的 Pod 偏斜

演示步骤：

```bash
kubectl -n exp-aliyun-k8s scale deploy/game-deployment --replicas=3
kubectl -n exp-aliyun-k8s get pods -o wide -l app=game
```

**预期结果 ✅**

- game Pod 分布在不同节点（`NODE` 列不同）

## 6. HPA 实验（参考 exp9）

```bash
kubectl -n exp-aliyun-k8s apply -f k8s/hpa.yaml
kubectl -n exp-aliyun-k8s get hpa
```

**预期结果 ✅**

- HPA 创建成功，`game-deployment` 会根据 CPU 自动扩缩

> HPA 依赖 metrics-server，如无请先安装。

### 6.1 压测脚本（触发 HPA）

已提供脚本：`scripts/hpa-loadtest.sh`

```bash
chmod +x scripts/hpa-loadtest.sh
./scripts/hpa-loadtest.sh http://127.0.0.1:18080 20 120
```

**预期结果 ✅**

- `kubectl -n exp-aliyun-k8s get hpa` 中的 `TARGETS` 接近或超过 50%
- `kubectl -n exp-aliyun-k8s get pods -l app=game` 显示副本数增加

## 7. VPA 实验

```bash
kubectl -n exp-aliyun-k8s apply -f k8s/vpa.yaml
kubectl -n exp-aliyun-k8s describe vpa game-vpa
```

**预期结果 ✅**

- VPA 输出资源建议（默认 `updateMode: Off`）
# 阿里云 K8s 演示（基于 exp_aliyun_docker 三层服务）

本页说明如何把三层服务（storage/game/gateway）构建为镜像并上传到本地镜像仓库（10.0.2.12:5000），以及如何让 Kubernetes 集群从该仓库拉取镜像。额外说明如何使用 10.0.2.12:5001 作为缓存代理（pull-through cache）。

## 目录结构

```text
exp_aliyun_k8s/
├── k8s/
│   ├── storage-deployment.yaml
│   ├── storage-service.yaml
│   ├── game-deployment.yaml
│   ├── game-service.yaml
│   ├── gateway-deployment.yaml
│   ├── gateway-service.yaml
│   ├── hpa.yaml
│   └── vpa.yaml
└── README.md
```

## 预设与目标

假设：

- 本地镜像仓库 A（直接 registry）：`10.0.2.12:5000/exp_aliyun_k8s`
- 缓存代理 registry（pull-through proxy）：`10.0.2.12:5001`

目标步骤（概览）：

1) 本地构建镜像并推送到 `10.0.2.12:5000/exp_aliyun_k8s`
2) 配置 Kubernetes 节点，使其能拉取私有仓库镜像（HTTP/insecure 或 containerd 配置）
3) 应用 `k8s/*.yaml`，Pod 会从 `10.0.2.12:5000/exp_aliyun_k8s` 拉取镜像

## 1. 构建并推送镜像到本地 registry

```bash
# 在项目根（包含 exp_aliyun_docker）运行：
export ACR=10.0.2.12:5000/exp_aliyun_k8s

docker build -f CourseCode/ch6/exp_aliyun_docker/docker/storage.Dockerfile -t ${ACR}/game-storage:v1 CourseCode/ch6/exp_aliyun_docker
docker build -f CourseCode/ch6/exp_aliyun_docker/docker/game.Dockerfile -t ${ACR}/game-service:v1 CourseCode/ch6/exp_aliyun_docker
docker build -f CourseCode/ch6/exp_aliyun_docker/docker/gateway.Dockerfile -t ${ACR}/game-gateway:v1 CourseCode/ch6/exp_aliyun_docker

docker push ${ACR}/game-storage:v1
docker push ${ACR}/game-service:v1
docker push ${ACR}/game-gateway:v1
```

说明：如果 registry 需要认证，请先 `docker login 10.0.2.12:5000`，或使用 `imagePullSecrets`（见下）。

## 2. 使用缓存代理（5001）

如果你已经在另一台机器上搭了 pull-through cache（代理）并监听 `10.0.2.12:5001`，节点只需要能访问该地址即可。当节点向 5001 拉取镜像且 5001 没有该镜像时，代理会去上游拉取并缓存。

示例（仅演示从代理拉取并转存到 5000）：

```bash
docker pull 10.0.2.12:5001/exp_aliyun_k8s/game-service:v1
docker tag 10.0.2.12:5001/exp_aliyun_k8s/game-service:v1 10.0.2.12:5000/exp_aliyun_k8s/game-service:v1
docker push 10.0.2.12:5000/exp_aliyun_k8s/game-service:v1
```

> 通常你直接 push 到 5000；代理 5001 会在节点拉取时自动从上游拿到并缓存。

## 3. 让 Kubernetes 节点能拉取私有 registry

### 3.1 Docker（HTTP/insecure）

如果 registry 使用 HTTP（非 TLS），Docker 引擎需要配置 insecure registry。在所有 k8s 节点上（master + worker）：

编辑或创建 `/etc/docker/daemon.json`：

```json
{
  "insecure-registries": ["10.0.2.12:5000", "10.0.2.12:5001"]
}
```

重启 Docker：

```bash
sudo systemctl restart docker
```

### 3.2 containerd

如果集群使用 containerd（常见于 k8s 1.24+），需要修改 `/etc/containerd/config.toml`（或 systemd drop-in）。示例配置片段（在 `plugins."io.containerd.grpc.v1.cri".registry` 下添加）：

```toml
[plugins."io.containerd.grpc.v1.cri".registry]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
      endpoint = ["https://registry-1.docker.io"]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."10.0.2.12:5000"]
      endpoint = ["http://10.0.2.12:5000"]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."10.0.2.12:5001"]
      endpoint = ["http://10.0.2.12:5001"]
```

重启 containerd：

```bash
sudo systemctl restart containerd
```

### 3.3 私有仓库认证（可选）

```bash
kubectl create secret docker-registry regcred \
  --docker-server=10.0.2.12:5000 \
  --docker-username=<user> --docker-password=<pw> --docker-email=<email> -n exp-aliyun-k8s
```

Deployment 里使用：

```yaml
imagePullSecrets:
  - name: regcred
```

## 4. 部署到 Kubernetes

```bash
kubectl create namespace exp-aliyun-k8s 2>/dev/null || true
kubectl -n exp-aliyun-k8s apply -f k8s/
kubectl -n exp-aliyun-k8s get pods,svc
```

## 5. 访问 Gateway（演示）

- NodePort：访问 `http://<NODE_IP>:30080`
- 或 port-forward：

```bash
kubectl -n exp-aliyun-k8s port-forward svc/gateway 18080:8080
```

## 6. Pod 反亲和性 + 拓扑分布约束实验

`k8s/game-deployment.yaml` 已加入：

- `podAntiAffinity`：尽量将 game Pod 分散到不同节点
- `topologySpreadConstraints`：限制同一节点的 Pod 偏斜

演示步骤：

```bash
kubectl -n exp-aliyun-k8s scale deploy/game-deployment --replicas=3
kubectl -n exp-aliyun-k8s get pods -o wide -l app=game
```

如果节点不足或被 cordon，Pod 可能 Pending（`DoNotSchedule`），这是预期行为。

## 7. HPA 实验（参考 exp9）

已新增 `k8s/hpa.yaml`，目标为 `game-deployment`，CPU 平均利用率 50% 触发扩缩。

```bash
kubectl -n exp-aliyun-k8s apply -f k8s/hpa.yaml
kubectl -n exp-aliyun-k8s get hpa
```

> HPA 依赖 metrics-server，如无请先安装。

## 8. VPA 实验

已新增 `k8s/vpa.yaml`，默认 `updateMode: Off`（只给出建议，不自动修改 Pod）。

```bash
kubectl -n exp-aliyun-k8s apply -f k8s/vpa.yaml
kubectl -n exp-aliyun-k8s describe vpa game-vpa
```

如需自动更新资源，可将 `updateMode` 改为 `Auto`（会触发 Pod 重建）。

## 常见问题

- 节点无法访问 registry（网络/防火墙）
- registry 使用 HTTP 且节点未配置 insecure registry
- 镜像标签或路径错误（确认 `10.0.2.12:5000/exp_aliyun_k8s/game-service:v1` 已存在）
- 私有仓库需要认证但未配置 imagePullSecrets

## 备注：Deployment 镜像地址

- `k8s/game-deployment.yaml` 使用 `10.0.2.12:5000/exp_aliyun_k8s/game-service:v1`
- `k8s/storage-deployment.yaml` 使用 `10.0.2.12:5000/exp_aliyun_k8s/game-storage:v1`
- `k8s/gateway-deployment.yaml` 使用 `10.0.2.12:5000/exp_aliyun_k8s/game-gateway:v1`

若希望从代理 5001 拉取，只需替换镜像前缀为 `10.0.2.12:5001/exp_aliyun_k8s/...` 或配置 containerd 将 5001 作为镜像源。
````markdown
# 阿里云 K8s 演示（基于 exp_aliyun_docker 三层服务）

## 目标

- 演示如何把三层服务（storage / game / gateway）打包成镜像并部署到 Kubernetes
- 演示常用操作：构建镜像、推送镜像、创建 namespace、apply manifests、端口访问/port-forward、查看日志与排查

## 目录结构

``text
exp_aliyun_k8s/
├── k8s/
│   ├── storage-deployment.yaml
│   ├── storage-service.yaml
+│   ├── game-deployment.yaml
# 阿里云 K8s 演示（基于 exp_aliyun_docker 三层服务）

本页详细说明如何把三层服务（storage/game/gateway）构建为镜像并上传到本地镜像仓库（10.0.2.12:5000），以及如何让 Kubernetes 集群从该仓库拉取镜像。额外说明如何使用 10.0.2.12:5001 作为缓存代理（pull-through cache）。

目录

``text
exp_aliyun_k8s/
├── k8s/
│   ├── storage-deployment.yaml   # Deployment -> 使用 10.0.2.12:5000/exp9/game-storage:v1
│   ├── storage-service.yaml
│   ├── game-deployment.yaml      # Deployment -> 使用 10.0.2.12:5000/exp9/game-service:v1
│   ├── game-service.yaml
│   ├── gateway-deployment.yaml   # Deployment -> 使用 10.0.2.12:5000/exp9/game-gateway:v1
│   └── gateway-service.yaml      # NodePort:30080 (示例)
├── build-and-deploy.sh           # 构建/推送/替换并 kubectl apply
└── README.md
```

假设：
- 本地镜像仓库 A (直接 registry)：10.0.2.12:5000/exp_aliyun_k8s
- 缓存代理 registry (pull-through proxy)：10.0.2.12:5001

目标步骤（概览）

1) 在本地构建镜像并推送到 `10.0.2.12:5000/exp_aliyun_k8s`
2) 配置 Kubernetes 节点，使其能从私有仓库拉取（如果是 HTTP，需要配置 insecure registry；如果使用 containerd，需要配置 containerd）
3) 在 k8s 中应用 `k8s/*.yaml`，Pod 会从 `10.0.2.12:5000/exp_aliyun_k8s` 拉取镜像

详细步骤

1) 构建并推送镜像到本地 registry

```bash
# 在项目根（包含 exp_aliyun_docker）运行：
export ACR=10.0.2.12:5000/exp_aliyun_k8s

docker build -f CourseCode/ch6/exp_aliyun_docker/docker/storage.Dockerfile -t ${ACR}/game-storage:v1 CourseCode/ch6/exp_aliyun_docker
docker build -f CourseCode/ch6/exp_aliyun_docker/docker/game.Dockerfile -t ${ACR}/game-service:v1 CourseCode/ch6/exp_aliyun_docker
docker build -f CourseCode/ch6/exp_aliyun_docker/docker/gateway.Dockerfile -t ${ACR}/game-gateway:v1 CourseCode/ch6/exp_aliyun_docker

docker push ${ACR}/game-storage:v1
docker push ${ACR}/game-service:v1
docker push ${ACR}/game-gateway:v1
```

说明：如果你的 registry 需要认证，请先 docker login 10.0.2.12:5000（或把 credentials 配置为 imagePullSecrets，见下）。

2) 使用缓存代理（5001）

- 如果你已经在另一台机器上搭了一个 pull-through cache（代理）并监听 10.0.2.12:5001，那么节点只需要能访问 10.0.2.12:5001 即可。当节点向 5001 拉取某个镜像且 5001 没有该镜像时，代理会去上游拉取并缓存。代理的配置通常在 registry 服务端（例如 Docker Registry + proxy 设置）。

示例：如果在节点上你想先从代理拉取镜像用于验证：

```bash
docker pull 10.0.2.12:5001/exp_aliyun_k8s/game-service:v1
docker tag 10.0.2.12:5001/exp_aliyun_k8s/game-service:v1 10.0.2.12:5000/exp_aliyun_k8s/game-service:v1
docker push 10.0.2.12:5000/exp_aliyun_k8s/game-service:v1
```

（上面只是示例，通常你直接把镜像 push 到 5000；代理 5001 会在节点拉取时自动从上游拿到并缓存）

3) 让 Kubernetes 节点能拉取私有 registry（HTTP/insecure 或 HTTPS）

如果 registry 是 HTTP（非 TLS），Docker 引擎需要配置 insecure registry。在所有 k8s 节点上（包括 master 与 worker）：

- 编辑或创建 `/etc/docker/daemon.json`，加入：

```json
{
	"insecure-registries": ["10.0.2.12:5000", "10.0.2.12:5001"]
}
```

然后重启 docker：

```bash
sudo systemctl restart docker
```

如果集群使用 containerd（常见于 k8s 1.24+ 或某些发行版），需要修改 containerd 的 `config.toml`（通常位于 `/etc/containerd/config.toml`）或通过 systemd drop-in。示例配置片段（在 `plugins."io.containerd.grpc.v1.cri".registry` 下添加）：

```toml
[plugins."io.containerd.grpc.v1.cri".registry]
	[plugins."io.containerd.grpc.v1.cri".registry.mirrors]
		[plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
			endpoint = ["https://registry-1.docker.io"]
		[plugins."io.containerd.grpc.v1.cri".registry.mirrors."10.0.2.12:5000"]
			endpoint = ["http://10.0.2.12:5000"]
		[plugins."io.containerd.grpc.v1.cri".registry.mirrors."10.0.2.12:5001"]
			endpoint = ["http://10.0.2.12:5001"]
```

编辑后重启 containerd：

```bash
sudo systemctl restart containerd
```

4) 如果 registry 需要认证（private）

- 在镜像仓库上使用用户名/密码（docker login）的方式：

```bash
docker login 10.0.2.12:5000
```

- 在 Kubernetes 中，你可以把认证信息放入 `imagePullSecret`：

```bash
kubectl create secret docker-registry regcred \
	--docker-server=10.0.2.12:5000 \
	--docker-username=<user> --docker-password=<pw> --docker-email=<email> -n exp-aliyun-k8s
```

然后在 Deployment 的 `spec.template.spec` 中使用：

```yaml
imagePullSecrets:
	- name: regcred
```

（本示例的 YAML 未配置 imagePullSecrets，若 registry 公开或节点已 login，可直接拉取）

5) 部署到 Kubernetes

```bash
kubectl create namespace exp-aliyun-k8s 2>/dev/null || true
kubectl -n exp-aliyun-k8s apply -f k8s/
kubectl -n exp-aliyun-k8s get pods,svc
```

6) 访问 Gateway（演示）

- 使用 NodePort（本示例 30080）：访问 `http://<NODE_IP>:30080`
- 或使用 port-forward：

```bash
kubectl -n exp-aliyun-k8s port-forward svc/gateway 18080:8080
# 打开 http://127.0.0.1:18080
```

示例：如果 Pod 卡在 ImagePullBackOff，请查看：

```bash
kubectl -n exp-aliyun-k8s describe pod <pod-name>
kubectl -n exp-aliyun-k8s logs <pod-name>
```

常见原因：
- 节点无法访问 registry（网络/防火墙）
- registry 使用 HTTP 且节点未配置 insecure registry
- 镜像标签或路径错误（请确认 `10.0.2.12:5000/exp_aliyun_k8s/game-service:v1` 已存在）
- 私有仓库需要认证但未配置 imagePullSecrets

附：K8s Deployment 已指向本地 registry

- `k8s/game-deployment.yaml` 使用镜像 `10.0.2.12:5000/exp_aliyun_k8s/game-service:v1`
- `k8s/storage-deployment.yaml` 使用镜像 `10.0.2.12:5000/exp_aliyun_k8s/game-storage:v1`
- `k8s/gateway-deployment.yaml` 使用镜像 `10.0.2.12:5000/exp_aliyun_k8s/game-gateway:v1`

如果你希望改为从代理 5001 拉取（使代理做缓存），只需把 manifests 中的镜像替换为 `10.0.2.12:5001/exp_aliyun_k8s/...` 或配置 containerd 将 5001 作为镜像镜像源。

---

## Pod 反亲和性（Anti-Affinity）与拓扑分布约束实验

本实验把 **game** 服务的 Pod 尽量分散到不同节点上（避免同机聚集），并使用拓扑分布约束确保多副本均匀分布。

已在 `k8s/game-deployment.yaml` 添加：

- `podAntiAffinity`：偏好不同 `kubernetes.io/hostname`
- `topologySpreadConstraints`：限制同一节点上 Pod 偏斜

演示步骤：

1) 将 `replicas` 调整为 2 或 3：

```bash
kubectl -n exp-aliyun-k8s scale deploy/game-deployment --replicas=3
```

2) 观察 Pod 分布：

```bash
kubectl -n exp-aliyun-k8s get pods -o wide -l app=game
```

如果节点不足或被 cordon，会触发 `DoNotSchedule`，Pod 会停在 Pending（这是预期行为）。

---

## HPA（水平自动伸缩）实验

已新增 `k8s/hpa.yaml`（参考 exp9 的 HPA 设置），目标是 `game-deployment`，并使用 CPU 利用率 50% 作为扩缩容指标。

应用 HPA：

```bash
kubectl -n exp-aliyun-k8s apply -f k8s/hpa.yaml
kubectl -n exp-aliyun-k8s get hpa
```

查看 HPA 伸缩过程：

```bash
kubectl -n exp-aliyun-k8s describe hpa game-hpa
```

提示：HPA 需要 metrics-server。若没有，需先安装 metrics-server。

---

## VPA（垂直自动伸缩）实验

已新增 `k8s/vpa.yaml`（默认 `updateMode: Off`，只给出资源建议，不会自动修改 Pod）。

应用 VPA：

```bash
kubectl -n exp-aliyun-k8s apply -f k8s/vpa.yaml
kubectl -n exp-aliyun-k8s describe vpa game-vpa
```

如需自动更新资源，可将 `updateMode` 改为 `Auto`（注意：会触发 Pod 重建）。

---

如果你愿意，我可以：

- 把 manifests 加上 `imagePullSecrets` 的示例并生成 `kubectl` 命令；
- 或把 `build-and-deploy.sh` 修改为默认推送到 `10.0.2.12:5001`（当作代理）并演示如何在节点验证缓存。
