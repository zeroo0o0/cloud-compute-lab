# 实验九：Node 级扩缩与云资源协同 (基础设施弹性)

## 对应教材

第 69-75 页

## 实验目标

验证基础设施层（Infrastructure Layer）的自动扩缩机制。通过 HPA（水平 Pod 自动扩缩容）演示在应用请求超出集群现有容量时的自动扩容行为，以及通过 PDB（Pod Disruption Budget）配置缩容保护。

> 本实验部署在阿里云 K8s 集群上，使用 HPA + behavior 配置实现快速扩缩容演示。
> 详细部署指南见 [deploy-aliyun.md](deploy-aliyun.md)。

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
│   ├── hpa.yaml             # HPA 配置 (CPU 50%, 1-6 副本, 演示加速)
│   └── cluster-autoscaler-sim.yaml  # 真实云环境 CA 配置说明
├── Dockerfile
├── deploy.sh                # 一键部署脚本
├── deploy-aliyun.md         # 阿里云部署指南
├── go.mod
└── README.md
```

## 前置条件

- 已完成 [集群搭建](../cluster-setup/README.md)
- 已配置 ACR 镜像仓库访问（ImagePullSecret）
- metrics-server 已安装

## 快速部署

```bash
cd CourseCode/ch6/exp9

# 一键部署（构建 + 推送 + 部署）
bash deploy.sh
```

## 手动部署

### 步骤 1：构建并推送镜像

```bash
cd CourseCode/ch6/exp9

# 构建镜像（注意 --platform linux/amd64）
docker build --platform linux/amd64 -t ch6-exp9-game:latest .

# 推送到 ACR
docker tag ch6-exp9-game:latest crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp9-game:latest
docker push crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp9-game:latest
```

### 步骤 2：部署服务

```bash
kubectl create namespace exp9
kubectl apply -f k8s/game-deployment.yaml
kubectl apply -f k8s/game-service.yaml
kubectl apply -f k8s/hpa.yaml
```

### 步骤 3：验证部署

```bash
# 查看 Pod 状态
kubectl -n exp9 get pods -o wide

# 查看 Service
kubectl -n exp9 get svc

# 查看 HPA
kubectl -n exp9 get hpa

# 访问服务（使用任意节点公网 IP）
export NODE_IP=<任意节点公网IP>
curl http://$NODE_IP:30080/health
```

### 步骤 4：压测触发 HPA 扩容

```bash
# 终端 1：持续监控
watch -n 2 kubectl -n exp9 get pods
watch -n 2 kubectl -n exp9 get hpa

# 终端 2：压测（20 并发，持续 120 秒）
cd CourseCode/ch6/exp9
go run cmd/loadtest/main.go -target http://$NODE_IP:30080 -c 20 -d 120s

# 或使用内置 stress 接口
for i in $(seq 1 20); do
  curl -s -X POST "http://$NODE_IP:30080/stress?duration=60s" &
done
```

### 步骤 5：观察扩缩容

```bash
# 观察 HPA 状态
kubectl -n exp9 describe hpa game-service-hpa

# 观察 Pod 数量变化
kubectl -n exp9 get pods -w
```

## 预期结果（演示加速后）

### 扩容阶段

| 时间 | Pod 数量 | CPU 使用率 | 事件 |
|------|---------|-----------|------|
| 0s   | 1       | >50%      | 压测开始 |
| 15s  | 4       | ~50%      | HPA 触发扩容（stabilizationWindow=0） |
| 30s  | 6       | ~50%      | 持续扩容到 maxReplicas |

### 缩容阶段

| 时间 | Pod 数量 | 事件 |
|------|---------|------|
| 0s   | 6       | 压测停止 |
| 30s  | 1       | 冷却窗口结束（stabilizationWindow=30s），一次性缩到最小 |

> 对比默认配置：扩容 30-50 秒，缩容 5-8 分钟。
> 优化后：扩容 ~15 秒，缩容 ~30 秒。

## 关键概念

- **HPA (Horizontal Pod Autoscaler)**：根据 CPU/内存使用率自动调整 Pod 副本数
- **behavior 字段**：控制扩缩容速度，`stabilizationWindowSeconds` 设置稳定窗口
- **Cluster Autoscaler**：当 Pod 因资源不足 Pending 时，自动向云平台申请新 Node
- **PDB (Pod Disruption Budget)**：限制同时中断的 Pod 数量，保护关键服务

## 清理

```bash
kubectl delete namespace exp9
```
