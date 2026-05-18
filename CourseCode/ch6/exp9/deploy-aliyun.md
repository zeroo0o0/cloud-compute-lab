# exp9 阿里云部署指南：Node 级扩缩与云资源协同

> 前置条件：已完成 [集群搭建](../cluster-setup/README.md)，3 台 ECS 组成 K8s 集群。

## 一、资源需求

| 项目 | 值 |
|------|-----|
| Namespace | `exp9` |
| Pod 数 | 1 → 10（HPA 自动扩缩） |
| 每 Pod CPU | 100m request / 500m limit |
| 每 Pod 内存 | 64Mi request / 256Mi limit |
| 特殊组件 | metrics-server（集群搭建时已安装） |
| 外部访问 | NodePort 30080 |

## 二、构建并推送镜像

```bash
cd CourseCode/ch6/exp9

# 构建镜像
docker build -t ch6-exp9-game:latest .

# 打 ACR 标签（替换为你的命名空间）
docker tag ch6-exp9-game:latest crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp9-game:latest

# 推送
docker push crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp9-game:latest
```

## 三、修改 K8s 配置

### 3.1 修改 game-deployment.yaml

将以下内容替换到 `k8s/game-deployment.yaml`：

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: game-service
  namespace: exp9
  labels:
    app: game-service
spec:
  replicas: 1
  selector:
    matchLabels:
      app: game-service
  template:
    metadata:
      labels:
        app: game-service
    spec:
      containers:
      - name: game
        image: crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp9-game:latest
        imagePullPolicy: Always
        ports:
        - containerPort: 8080
        resources:
          requests:
            cpu: "100m"
            memory: "64Mi"
          limits:
            cpu: "500m"
            memory: "256Mi"
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 2
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
```

### 3.2 修改 game-service.yaml

```yaml
apiVersion: v1
kind: Service
metadata:
  name: game-service
  namespace: exp9
spec:
  type: NodePort
  selector:
    app: game-service
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30080
```

### 3.3 修改 hpa.yaml

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: game-service-hpa
  namespace: exp9
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: game-service
  minReplicas: 1
  maxReplicas: 6
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 50
  # === 演示加速配置 ===
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 0    # 不等待稳定窗口，立即扩容
      policies:
      - type: Pods
        value: 4                        # 每次最多增加 4 个 Pod
        periodSeconds: 15               # 每 15 秒可执行一次
    scaleDown:
      stabilizationWindowSeconds: 30    # 缩容冷却从 5 分钟降到 30 秒
      policies:
      - type: Percent
        value: 100                      # 允许一次性缩到最小
        periodSeconds: 30               # 每 30 秒可执行一次
```

> `maxReplicas` 设为 6 而非 10，因为 3 节点 × 2 Pod/节点 = 6，避免资源争抢影响其他实验。
>
> `behavior` 字段是演示加速的关键配置：
> - `scaleUp.stabilizationWindowSeconds: 0` — 扩容不等待，CPU 超标立即触发
> - `scaleDown.stabilizationWindowSeconds: 30` — 缩容冷却从默认 5 分钟降到 30 秒

## 四、部署

```bash
# 创建 Namespace
kubectl create namespace exp9

# 部署所有资源
kubectl apply -f k8s/game-deployment.yaml
kubectl apply -f k8s/game-service.yaml
kubectl apply -f k8s/hpa.yaml

# 等待 Pod 就绪
kubectl -n exp9 wait --for=condition=ready pod -l app=game-service --timeout=60s

# 验证
kubectl -n exp9 get pods
kubectl -n exp9 get hpa
```

## 五、验证实验

### 5.1 获取访问地址

```bash
# 方法 1：使用任意节点的公网 IP
export NODE_IP=<任意节点公网IP>

# 方法 2：自动获取
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="ExternalIP")].address}')
# 如果没有 ExternalIP（kubeadm 部署通常没有），使用节点的公网 IP
```

### 5.2 测试服务

```bash
# 健康检查
curl http://$NODE_IP:30080/health

# 游戏接口
curl http://$NODE_IP:30080/move?player=test\&dir=north
```

### 5.3 压测触发 HPA 扩容

```bash
# 终端 1：持续监控
watch -n 2 kubectl -n exp9 get pods
watch -n 2 kubectl -n exp9 get hpa

# 终端 2：压测（本机执行，需安装 Go）
cd CourseCode/ch6/exp9
go run cmd/loadtest/main.go -target http://$NODE_IP:30080 -c 20 -d 120s

# 或使用内置 stress 接口（不需要 Go 环境）
for i in $(seq 1 20); do
  curl -s -X POST "http://$NODE_IP:30080/stress?duration=60s" &
done
```

### 5.4 观察扩容过程

```bash
# HPA 状态
kubectl -n exp9 describe hpa game-service-hpa

# 预期输出：
# Targets:  <unknown>/50% (avg CPU)
# Events:
#   Successfully scaled to 2 replicas
#   Successfully scaled to 3 replicas
#   ...

# Pod 列表
kubectl -n exp9 get pods -o wide
# NAME                            READY   STATUS    RESTARTS   AGE
# game-service-7d8b6c5f4-abc12   1/1     Running   0          30s
# game-service-7d8b6c5f4-def34   1/1     Running   0          15s
# game-service-7d8b6c5f4-ghi56   1/1     Running   0          10s
```

### 5.5 观察缩容

```bash
# 停止压测后，等待约 30 秒冷却时间（已通过 behavior 配置加速）
# HPA 会自动缩减 Pod 数量
watch -n 2 kubectl -n exp9 get pods
```

## 六、预期结果（演示加速后）

### 扩容时间线

| 时间 | 事件 | Pod 数 |
|------|------|--------|
| 0s | 压测开始，CPU 瞬间打满 | 1 |
| 15s | HPA 检测到 CPU > 50%，触发扩容 | 1→4 |
| 30s | 新 Pod 就绪，负载分散 | 4 |
| 45s | CPU 仍高，继续扩容 | 4→6 |
| 60s | 负载均衡，CPU 降至 50% 以下 | 6（稳定） |

### 缩容时间线

| 时间 | 事件 | Pod 数 |
|------|------|--------|
| 0s | 停止压测，CPU 降至 ~0% | 6 |
| 15s | HPA 检测到 CPU < 50% | 6 |
| 30s | 冷却窗口结束，触发缩容 | 6→1 |
| 45s | 多余 Pod 终止 | 1（恢复） |

> **演示关键**：优化后扩容 ~15-30 秒可见，缩容 ~30-45 秒可见。
> 对比默认配置：扩容 30-50 秒，缩容 5-8 分钟。

### 未优化 vs 优化对比

| 配置项 | 默认值 | 优化值 | 效果 |
|--------|--------|--------|------|
| `stabilizationWindowSeconds` (down) | 300s (5分钟) | 30s | 缩容等待从 5 分钟降到 30 秒 |
| `stabilizationWindowSeconds` (up) | 0s | 0s | 扩容本身已经很快 |
| readinessProbe `initialDelaySeconds` | 2s | 1s | Pod 更快标记为就绪 |
| readinessProbe `periodSeconds` | 5s | 2s | 探测更频繁 |

## 七、清理

```bash
kubectl delete namespace exp9
```
