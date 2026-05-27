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

## 0. 使用 kubeadm 初始化集群（第一次搭建）

### 0.1 控制平面初始化（k8s-a）

这里仅展示最小化的配置，实际生产环境需要更多安全/网络配置。
```bash
kubeadm init \
  --apiserver-advertise-address=<MASTER_IP> \
  --pod-network-cidr=10.244.0.0/16

配置 kubectl：

```bash
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config

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

## 6. 无状态网关与无感切换（参考 exp8）

目标：网关不保存玩家状态，断线后客户端自动重连即可继续使用。

演示步骤：

```bash
# 网关扩容到 2 个副本（让 Service 负载均衡）
kubectl -n exp-aliyun-k8s scale deploy/gateway-deployment --replicas=2
kubectl -n exp-aliyun-k8s get pods -l app=gateway -o wide
```

启动客户端（可在任意机器执行）：

```bash
CLIENT_SERVER_URL="<NODE_IP>:30080" \
  go run CourseCode/ch6/exp_aliyun_docker/cmd/client
```

> 可选：若有多个入口地址，可使用 `CLIENT_SERVER_URLS="addr1,addr2"` 自动切换。

模拟网关故障：

```bash
kubectl -n exp-aliyun-k8s delete pod -l app=gateway --force --grace-period=0
```

**预期结果 ✅**

- 客户端短暂重试后继续工作（无感切换到新网关）
- `gateway` 仍保持 2 副本（Deployment 会自动拉起）

## 7. Pod 反亲和性 + 拓扑分布约束实验

`k8s/game-deployment.yaml` 已加入：

- `podAntiAffinity`：尽量将 game Pod 分散到不同节点
- `topologySpreadConstraints`：限制同一节点的 Pod 偏斜

演示步骤：

```bash
kubectl -n exp-aliyun-k8s scale deploy/game-deployment --replicas=3
kubectl -n exp-aliyun-k8s get pods -o wide -l app=game
```

如果节点不足或被 cordon，Pod 可能 Pending（`DoNotSchedule`），这是预期行为。

## 8. HPA 实验（参考 exp9）

已新增 `k8s/hpa.yaml`，目标为 `game-deployment`，CPU 平均利用率 50% 触发扩缩。

```bash
kubectl -n exp-aliyun-k8s apply -f k8s/hpa.yaml
kubectl -n exp-aliyun-k8s get hpa
```

> HPA 依赖 metrics-server，如无请先安装。

已提供脚本：`scripts/hpa-loadtest.sh`

```bash
chmod +x scripts/hpa-loadtest.sh
./scripts/hpa-loadtest.sh http://127.0.0.1:18080 20 120
```

**预期结果 ✅**

- `kubectl -n exp-aliyun-k8s get hpa` 中的 `TARGETS` 接近或超过 50%
- `kubectl -n exp-aliyun-k8s get pods -l app=game` 显示副本数增加

## 9. VPA 实验

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






