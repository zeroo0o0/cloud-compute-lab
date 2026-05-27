# 阿里云 K8s 演示（基于 exp_aliyun_docker 三层服务）

## 目标

- kubeadm 初始化控制平面与加入工作节点
- 构建镜像并推送到本地镜像仓库
- 部署三层服务（storage / game / gateway）到 K8s 并验证访问
- 反亲和性与拓扑分布约束实验
- HPA 水平扩缩容实验
- VPA 垂直资源推荐实验

> **重点提示**：以下步骤按"第一次搭建集群"的流程组织，不包含节点删除/重加入的内容。

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
├── scripts/
│   └── hpa-loadtest.sh
└── README.md
```

## 1. 使用 kubeadm 初始化集群（第一次搭建）

### 1.1 控制平面初始化（k8s-a）

最小化配置，实际生产环境需要更多安全/网络配置：

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

**预期结果**

- `kubectl get nodes` 能看到控制平面节点，状态为 `Ready`

### 1.2 工作节点加入集群（k8s-b / k8s-c / k8s-d）

在控制平面生成 join 命令：

```bash
kubeadm token create --print-join-command
```

在每个工作节点执行输出的 join 命令：

```bash
kubeadm join <MASTER_IP>:6443 --token <TOKEN> --discovery-token-ca-cert-hash sha256:<HASH>
```

**预期结果**

- `kubectl get nodes` 可看到 4 个节点全部 `Ready`

## 2. 构建并推送镜像到本地 registry

本地镜像仓库地址：`10.0.2.12:5000/exp_aliyun_k8s`

```bash
export ACR=10.0.2.12:5000/exp_aliyun_k8s

docker build -f CourseCode/ch6/exp_aliyun_docker/docker/storage.Dockerfile -t ${ACR}/game-storage:v1 CourseCode/ch6/exp_aliyun_docker
docker build -f CourseCode/ch6/exp_aliyun_docker/docker/game.Dockerfile -t ${ACR}/game-service:v1 CourseCode/ch6/exp_aliyun_docker
docker build -f CourseCode/ch6/exp_aliyun_docker/docker/gateway.Dockerfile -t ${ACR}/game-gateway:v1 CourseCode/ch6/exp_aliyun_docker

docker push ${ACR}/game-storage:v1
docker push ${ACR}/game-service:v1
docker push ${ACR}/game-gateway:v1
```

**预期结果**

- registry 中出现 `game-storage` / `game-service` / `game-gateway` 镜像

## 3. 让节点能够拉取私有镜像（HTTP / insecure）

如果 registry 使用 HTTP（非 TLS），需要在每个节点配置 insecure registry。

### 3.1 Docker

在 `/etc/docker/daemon.json` 中添加：

```json
{
  "insecure-registries": ["10.0.2.12:5000"]
}
```

重启 Docker：

```bash
sudo systemctl restart docker
```

### 3.2 containerd

如果集群使用 containerd（常见于 k8s 1.24+），需要修改 `/etc/containerd/config.toml`。在 `plugins."io.containerd.grpc.v1.cri".registry` 下添加：

```toml
[plugins."io.containerd.grpc.v1.cri".registry]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
      endpoint = ["https://registry-1.docker.io"]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."10.0.2.12:5000"]
      endpoint = ["http://10.0.2.12:5000"]
```

重启 containerd：

```bash
sudo systemctl restart containerd
```

### 3.3 私有仓库认证（可选）

```bash
kubectl create secret docker-registry regcred \
  --docker-server=10.0.2.12:5000 \
  --docker-username=<user> --docker-password=<pw> --docker-email=<email> \
  -n exp-aliyun-k8s
```

Deployment 中使用：

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

**预期结果**

- 3 个 Deployment 各 1 个副本，Pod 状态为 `Running`
- 3 个 Service 已创建

## 5. 访问 Gateway

通过 NodePort 访问：`http://<NODE_IP>:30080`

或使用 port-forward：

```bash
kubectl -n exp-aliyun-k8s port-forward svc/gateway 18080:8080
```

## 6. Pod 反亲和性 + 拓扑分布约束实验

`k8s/game-deployment.yaml` 已配置：

- `podAntiAffinity`（preferred）：尽量将 game Pod 分散到不同节点
- `topologySpreadConstraints`：限制同一节点的 Pod 偏斜（maxSkew=1）

```bash
kubectl -n exp-aliyun-k8s scale deploy/game-deployment --replicas=3
kubectl -n exp-aliyun-k8s get pods -o wide -l app=game
```

**预期结果**

- 3 个 game Pod 尽量分散到不同节点
- 如果节点不足，Pod 可能 Pending（`DoNotSchedule`）或集中调度（`ScheduleAnyway`），取决于配置

## 7. HPA 实验

已配置 `k8s/hpa.yaml`，目标为 `game-deployment`，CPU 平均利用率 30% 触发扩容，最大 12 副本。

### 7.1 部署 HPA

```bash
kubectl -n exp-aliyun-k8s apply -f k8s/hpa.yaml
kubectl -n exp-aliyun-k8s get hpa
```

> HPA 依赖 metrics-server，如未安装请先部署。

### 7.2 施加负载

使用提供的脚本进行压测：

```bash
chmod +x scripts/hpa-loadtest.sh
./scripts/hpa-loadtest.sh http://127.0.0.1:18080 20 120
```

参数说明：`目标地址 并发数 持续秒数`

> 脚本会在指定秒数后自动退出并打印统计信息。如果出现卡住不退出的情况，按 `Ctrl+C` 终止即可。

### 7.3 观测效果

在压测期间另开终端观察：

```bash
# 查看 HPA 状态（观察 TARGETS 列的 CPU 利用率）
kubectl -n exp-aliyun-k8s get hpa -w

# 查看副本数变化
kubectl -n exp-aliyun-k8s get pods -l app=game -w

# 查看事件（可看到扩容/缩容原因）
kubectl -n exp-aliyun-k8s describe hpa game-hpa
```

**预期结果**

- `kubectl get hpa` 中 `TARGETS` 列显示 CPU 利用率超过 30%
- game Pod 副本数从 1 逐步增加（最多到 12）
- 压测结束后约 15 秒（stabilizationWindow），副本数自动回落

### 7.4 清理 HPA

```bash
kubectl -n exp-aliyun-k8s delete hpa game-hpa
```

清理后 game-deployment 副本数不会自动回缩，如需手动恢复：

```bash
kubectl -n exp-aliyun-k8s scale deploy/game-deployment --replicas=1
```

## 8. VPA 实验

VPA（Vertical Pod Autoscaler）根据历史资源使用情况，为 Pod 推荐合适的 CPU / Memory 的 requests 和 limits。

已配置 `k8s/vpa.yaml`，默认 `updateMode: Off`（仅给出建议，不自动修改 Pod）。

### 8.1 安装 VPA 组件

如果集群中尚未安装 VPA，需要先部署 VPA（包含 recommender、updater、admission-controller）：

```bash
# 克隆 VPA 仓库
git clone https://github.com/kubernetes/autoscaler.git
cd autoscaler/vertical-pod-autoscaler

# 部署 VPA 组件
./hack/vpa-up.sh

# 验证安装
kubectl get pods -n kube-system | grep vpa
```

预期看到 `vpa-recommender`、`vpa-updater`、`vpa-admission-controller` 三个 Pod 运行。

> 如果使用云厂商托管 K8s（如阿里云 ACK），VPA 可能已内置，跳过此步骤。

### 8.2 部署 VPA

```bash
kubectl -n exp-aliyun-k8s apply -f k8s/vpa.yaml
```

### 8.3 观测 VPA 推荐值

查看 VPA 为 game-deployment 给出的资源推荐：

```bash
kubectl -n exp-aliyun-k8s describe vpa game-vpa
```

输出中关注 `Recommendation` 部分：

```text
Status:
  Recommendation:
    Container Recommendations:
      Container Name:  game
      Lower Bound:     # 资源下限（低于此值 Pod 大概率被 OOMKill 或 CPU 被限流）
        Cpu:     50m
        Memory:  64Mi
      Target:          # VPA 推荐的 requests 值（建议设置为这个值）
        Cpu:     125m
        Memory:  150Mi
      Upper Bound:     # 资源上限（建议 limits 不低于此值）
        Cpu:     500m
        Memory:  512Mi
      Uncapped Target: # 不考虑 min/max 约束时的纯推荐值
        Cpu:     125m
        Memory:  150Mi
```

各字段含义：

| 字段 | 含义 |
|------|------|
| **Lower Bound** | 低于此值，Pod 大概率出现资源不足（OOMKill / CPU throttling） |
| **Target** | VPA 推荐设置的 `requests`，是最有参考价值的值 |
| **Upper Bound** | 建议 `limits` 不低于此值，否则可能影响正常运行 |
| **Uncapped Target** | 不受 `minAllowed` / `maxAllowed` 限制的纯算法推荐 |

### 8.4 持续观测资源推荐变化

```bash
# 每 5 秒刷新一次 VPA 推荐值
watch -n 5 'kubectl -n exp-aliyun-k8s describe vpa game-vpa | grep -A 20 "Recommendation"'
```

同时开另一个终端施加负载，观察推荐值是否随负载变化：

```bash
# 先让 game 服务处理一些请求
./scripts/hpa-loadtest.sh http://127.0.0.1:18080 20 60
```

**预期结果**

- 刚部署时，VPA 推荐值基于默认资源（requests: cpu=100m, memory=128Mi）
- 短时间压测后，`Target` 可能不会立即变化——VPA recommender 默认需要数小时的数据才会调整推荐值
- 如果需要在课堂上演示推荐值变化，可以先将 game-deployment 的 `requests` 故意设得很低（如 cpu=10m, memory=32Mi），观察 VPA 推荐值始终高于当前值，说明 VPA 识别出资源不足
- `Lower Bound` 和 `Upper Bound` 会随 `Target` 同步变化

### 8.5 切换为 Auto 模式（自动调整资源）

将 `updateMode` 改为 `Auto`，VPA 会自动修改 Pod 的资源配额（会触发 Pod 重建）：

```yaml
# k8s/vpa.yaml
updatePolicy:
  updateMode: "Auto"
```

```bash
kubectl -n exp-aliyun-k8s apply -f k8s/vpa.yaml
```

**预期结果**

- VPA 检测到当前 Pod 的资源 requests 与推荐值差距较大时，会 evict 旧 Pod
- 新 Pod 使用 VPA 推荐的 `requests` 值启动
- `kubectl get pods -l app=game -w` 可观察到 Pod 被重建

> **注意**：`Auto` 模式会导致 Pod 重建，生产环境慎用。建议先在 `Off` 模式下观察推荐值，手动调整后再切回 `Off`。

### 8.6 清理 VPA

```bash
kubectl -n exp-aliyun-k8s delete vpa game-vpa
```

如需卸载 VPA 组件：

```bash
cd autoscaler/vertical-pod-autoscaler
./hack/vpa-down.sh
```

## 9. 清理

```bash
kubectl -n exp-aliyun-k8s delete -f k8s/
kubectl delete namespace exp-aliyun-k8s
```

## 常见问题

| 问题 | 排查 |
|------|------|
| 节点无法访问 registry | 检查网络/防火墙，确认 insecure registry 已配置 |
| 镜像拉取失败 | 确认镜像标签和路径，如 `10.0.2.12:5000/exp_aliyun_k8s/game-service:v1` |
| 私有仓库认证失败 | 配置 `imagePullSecrets` 或在节点上 `docker login` |
| HPA 无数据 | 确认 metrics-server 已部署：`kubectl top pods` |
| VPA 无推荐值 | 确认 VPA recommender 正在运行，且 Pod 有实际负载 |
| VPA Auto 模式不生效 | 确认 `vpa-updater` 和 `vpa-admission-controller` 均正常运行 |

## 备注：Deployment 镜像地址

| 服务 | 镜像 |
|------|------|
| storage | `10.0.2.12:5000/exp_aliyun_k8s/game-storage:v1` |
| game | `10.0.2.12:5000/exp_aliyun_k8s/game-service:v1` |
| gateway | `10.0.2.12:5000/exp_aliyun_k8s/game-gateway:v1` |
