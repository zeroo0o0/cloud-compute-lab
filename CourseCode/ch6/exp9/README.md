# 实验九：Node 级扩缩与云资源协同 (基础设施弹性)

## 对应教材

第 69-75 页

## 实验目标

验证基础设施层（Infrastructure Layer）的自动扩缩机制。通过 HPA（水平 Pod 自动扩缩容）演示在应用请求超出集群现有容量时的自动扩容行为，以及通过 PDB（Pod Disruption Budget）配置缩容保护。

> **注意**：minikube 是单节点环境，无法真正模拟 Cluster Autoscaler 的"自动买机器"行为。
> 本实验使用 HPA 模拟自动扩缩的核心概念，真实云环境部署方案见 `k8s/cluster-autoscaler-sim.yaml`。

## 目录结构

```
exp9/
├── cmd/
│   ├── game/
│   │   └── main.go          # Game 服务：/health, /move, /stress 接口
│   └── loadtest/
│       └── main.go          # 压测客户端：并发请求 + 实时统计
├── k8s/
│   ├── game-deployment.yaml # Game Deployment (HPA 目标)
│   ├── game-service.yaml    # NodePort Service (30080)
│   ├── hpa.yaml             # HPA 配置 (CPU 50%, 1-10 副本)
│   └── cluster-autoscaler-sim.yaml  # 真实云环境 CA 配置说明
├── Dockerfile
├── go.mod
└── README.md
```

## 前置条件

```bash
# 1. 启动 minikube
minikube start --cpus=4 --memory=4096

# 2. 启用 metrics-server
minikube addons enable metrics-server

# 3. 配置 Docker 使用 minikube 的 Docker daemon
eval $(minikube docker-env)
```

## 运行步骤

### 步骤 1：构建镜像

```bash
cd CourseCode/ch6/exp9
docker build -t ch6-exp9-game:latest .
```

### 步骤 2：部署服务

```bash
kubectl apply -f k8s/game-deployment.yaml
kubectl apply -f k8s/game-service.yaml
kubectl apply -f k8s/hpa.yaml
```

### 步骤 3：验证部署

```bash
# 查看 Pod 状态
kubectl get pods -l app=game-service -o wide

# 查看 Service
kubectl get svc game-service

# 查看 HPA
kubectl get hpa game-service-hpa

# 访问服务
curl http://$(minikube ip):30080/health
```

### 步骤 4：压测触发 HPA 扩容

```bash
# 开启新终端，持续监控
watch kubectl get pods -l app=game-service
watch kubectl get hpa game-service-hpa

# 启动压测（20 并发，持续 120 秒）
go run cmd/loadtest/main.go -target http://$(minikube ip):30080 -c 20 -d 120s

# 或者使用内置 stress 接口直接打满 CPU
curl -X POST "http://$(minikube ip):30080/stress?duration=60s"
```

### 步骤 5：观察扩容

```bash
# 观察 Pod 数量变化（从 1 逐步增加）
kubectl get pods -l app=game-service -w

# 观察 HPA 状态
kubectl describe hpa game-service-hpa
```

### 步骤 6：停止压测，观察缩容

```bash
# 停止所有压测请求后，等待 5 分钟冷却时间
# HPA 会自动将 Pod 数量缩减回 minReplicas=1
watch kubectl get pods -l app=game-service
```

## 预期结果

### 扩容阶段

| 时间 | Pod 数量 | CPU 使用率 | 事件 |
|------|---------|-----------|------|
| 0s   | 1       | ~0%       | 初始状态 |
| 15s  | 1       | >50%      | HPA 检测到 CPU 过载 |
| 30s  | 2-3     | ~50%      | HPA 触发扩容 |
| 60s  | 4-6     | ~50%      | 持续扩容直到 CPU 降至目标 |

### 缩容阶段

| 时间 | Pod 数量 | 事件 |
|------|---------|------|
| 0s   | 6       | 压测停止 |
| 300s | 6       | 等待冷却时间（默认 5 分钟） |
| 360s | 3       | HPA 开始缩容 |
| 600s | 1       | 回到最小副本数 |

## 关键概念

- **HPA (Horizontal Pod Autoscaler)**：根据 CPU/内存使用率自动调整 Pod 副本数
- **Cluster Autoscaler**：当 Pod 因资源不足 Pending 时，自动向云平台申请新 Node
- **PDB (Pod Disruption Budget)**：限制同时中断的 Pod 数量，保护关键服务
- **Scale-to-Zero**：HPA 可将副本数缩至 0（需配置 `--horizontal-pod-autoscaler-scale-down-to-zero`）

## 真实云环境部署

参考 `k8s/cluster-autoscaler-sim.yaml` 中的说明，配置对应云平台的 Cluster Autoscaler：

- 阿里云 ACK：在集群配置中开启节点自动伸缩
- AWS EKS：安装 cluster-autoscaler 并配置 IAM
- GCP GKE：`gcloud container clusters update --enable-autoscaling`
