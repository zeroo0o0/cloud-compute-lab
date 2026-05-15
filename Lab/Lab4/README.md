# Lab 4 · 云原生部署：Docker + Kubernetes

> **总分：15 分**
>
> **前置要求**：已完成 Lab 3，理解分布式集群架构。Lab 4 在 Lab 3 的基础上，将游戏系统容器化并部署到 Kubernetes 集群。

---

## 一、实验目标

本实验目标是把 Lab3 的"单进程多节点"架构，改造为真正的云原生部署：

**Phase 1：单体容器化（基础）**
- 将游戏服务器打包为 Docker 镜像
- 使用 K8s Deployment 部署到 minikube
- 通过 NodePort Service 暴露网关端口
- 使用 ConfigMap 外部化配置
- 使用 PVC 持久化游戏数据

**Phase 2：微服务拆分（进阶）**
- 将网关和节点拆分为独立 Pod
- 使用 StatefulSet 管理有状态游戏节点
- 通过 Headless Service 实现服务发现
- 节点间通过 JSON-over-TCP RPC 通信

---

## 二、实验背景

Lab3 中，所有组件（网关 + 3 个节点）运行在同一进程内，节点地址硬编码为 `127.0.0.1`。这种设计存在以下问题：

- 无法独立扩展单个组件
- 无法利用容器编排的自动恢复能力
- 无法演示真实云环境的服务发现和负载均衡
- 配置硬编码，无法适配不同部署环境

因此在 Lab4 中，需要将系统改造为容器化架构，使其能在 Kubernetes 上运行。

---

## 三、实验要求

### Phase 1：单体容器化（8 分）

1. 修改代码，将硬编码的地址和配置改为环境变量可配置（3 分）
2. 编写 Dockerfile，使用多阶段构建优化镜像大小（2 分）
3. 编写 K8s 配置清单，包含 Namespace、ConfigMap、PVC、Deployment、Service（3 分）

### Phase 2：微服务拆分（7 分）

4. 实现节点 RPC 协议，使网关能通过网络调用远程节点（3 分）
5. 编写 StatefulSet 和 Headless Service 配置（2 分）
6. 实现一键部署和验证脚本（2 分）

---

## 四、环境准备

### 1. 安装 minikube

```bash
# Windows (WSL2)
curl -LO https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64
sudo install minikube-linux-amd64 /usr/local/bin/minikube

# 或使用 brew (macOS)
brew install minikube
```

### 2. 安装 kubectl

```bash
# Linux
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install kubectl /usr/local/bin/kubectl

# 或使用包管理器
sudo apt install kubectl  # Ubuntu
brew install kubectl      # macOS
```

### 3. 启动 minikube

```bash
minikube start --cpus=2 --memory=2048
```

---

## 五、文件结构

```text
Lab4/
├── README.md
├── student/
│   ├── Dockerfile
│   ├── go.mod
│   ├── protocol/
│   │   ├── message.go          # 修改: GatewayAddr 可配置
│   │   └── node_rpc.go         # 新增: 节点 RPC 协议
│   ├── storage/store.go        # 不变
│   ├── world/world.go          # 不变
│   ├── cluster/
│   │   ├── cluster.go          # 修改: 接受配置结构体
│   │   └── remote_node.go      # 新增: 远程节点 RPC 客户端
│   ├── node/
│   │   └── server.go           # 新增: 节点 RPC 服务端
│   └── cmd/
│       ├── server/main.go      # 修改: 支持角色配置
│       ├── gateway/main.go     # 新增: 网关独立入口
│       ├── node/main.go        # 新增: 节点独立入口
│       ├── client/main.go      # 修改: 地址可配置
│       └── admin/main.go       # 修改: 地址可配置
├── k8s/
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── pvc.yaml
│   ├── deployment.yaml         # Phase 1
│   ├── service.yaml            # Phase 1
│   ├── gateway-deployment.yaml # Phase 2
│   ├── node-statefulset.yaml   # Phase 2
│   └── headless-service.yaml   # Phase 2
├── deploy.sh
├── verify.sh
├── undeploy.sh
└── test/
    └── autotest_k8s.go
```

---

## 六、核心配置项

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `ROLE` | `all` | 运行角色：`all`（单体）、`gateway`、`node` |
| `NODE_ID` | - | 节点 ID（node 模式必需） |
| `GATEWAY_ADDR` | `0.0.0.0:9310` | 网关监听地址 |
| `NODE_ADDRS` | `node-a=127.0.0.1:9311,...` | 节点地址列表 |
| `MAP_ASSIGNMENTS` | `green=node-a:node-c,...` | 地图-节点分配 |
| `NODE_MODE` | `local` | 节点模式：`local` 或 `remote` |
| `LAB3_DATA_ROOT` | `.` | 数据存储根目录 |

---

## 七、K8s 资源说明

### Namespace
资源隔离，所有 battleworld 资源在 `battleworld` 命名空间下。

### ConfigMap
存储游戏配置，通过环境变量注入容器。修改配置只需更新 ConfigMap 并重启 Pod。

### PersistentVolumeClaim
持久化用户数据和检查点。使用 `ReadWriteOnce` 模式，适合 minikube 单节点环境。

### Deployment
部署游戏服务器。包含健康检查（TCP Probe）、资源限制、环境变量注入。

### Service (NodePort)
将网关端口暴露到集群外部。客户端通过 `$(minikube ip):30310` 连接。

### StatefulSet（Phase 2）
管理有状态的游戏节点。每个 Pod 有稳定网络标识（node-0, node-1, node-2）。

### Headless Service（Phase 2）
为 StatefulSet 提供 DNS 服务发现。网关通过 `battleworld-node-0.battleworld-nodes:9311` 访问节点。

---

## 八、运行方式

### Phase 1：一键部署

```bash
cd Lab4
bash deploy.sh
```

### 连接游戏

```bash
cd Lab4/student
go run ./cmd/client
# 选择 2. 连接指定网关
# IP: $(minikube ip)
# 端口: 30310
```

### 管理命令

```bash
cd Lab4/student
go run ./cmd/admin 状态 $(minikube ip):30310
```

### 验证部署

```bash
bash verify.sh
```

### 清理资源

```bash
bash undeploy.sh
```

---

## 九、评分标准

| 任务 | 内容 | 分值 |
|------|------|------|
| 4-1 | 代码配置外部化 | 3 分 |
| 4-2 | Dockerfile 编写 | 2 分 |
| 4-3 | K8s 配置清单 | 3 分 |
| 4-4 | 节点 RPC 协议实现 | 3 分 |
| 4-5 | StatefulSet + Headless Service | 2 分 |
| 4-6 | 部署脚本 | 2 分 |
| **合计** | | **15 分** |

---

## 十、调试建议

### 查看 Pod 状态

```bash
kubectl -n battleworld get pods -o wide
```

### 查看 Pod 日志

```bash
kubectl -n battleworld logs -l app=battleworld --tail=50
```

### 进入 Pod 调试

```bash
kubectl -n battleworld exec -it deployment/battleworld -- sh
```

### 查看 Service

```bash
kubectl -n battleworld get svc
```

### 获取 minikube IP

```bash
minikube ip
```

### 端口转发（备选方案）

```bash
kubectl -n battleworld port-forward svc/battleworld-gateway 9310:9310
```
