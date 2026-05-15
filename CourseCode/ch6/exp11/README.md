# 实验十一：探针检测与故障自愈

## 对应教材

第 63-67 页（弹性-故障自愈）

## 实验目标

演示即使程序出现 Bug 导致进程假死，K8s 的 Liveness Probe 也能自动检测并重启容器，实现"永不宕机"。

## 目录结构

```
exp11/
├── cmd/
│   └── server/
│       └── main.go          # Game 服务：/health, /deadlock, /recover 接口
├── k8s/
│   ├── game-deployment.yaml # Deployment (含 Liveness Probe)
│   └── game-service.yaml    # NodePort Service (30080)
├── Dockerfile
├── go.mod
└── README.md
```

## 运行步骤

### 步骤 1：构建镜像

```bash
cd CourseCode/ch6/exp11
eval $(minikube docker-env)
docker build -t ch6-exp11-game:latest .
```

### 步骤 2：部署服务

```bash
kubectl apply -f k8s/
```

### 步骤 3：验证正常运行

```bash
# 查看 Pod 状态（应该显示 Running, RESTARTS: 0）
kubectl get pods -l app=game-service

# 访问健康接口
curl http://$(minikube ip):30080/health

# 查看服务状态
curl http://$(minikube ip):30080/status
```

### 步骤 4：模拟进程假死（制造车祸）

```bash
# 发送死锁指令，让 /health 返回 500
curl -X POST http://$(minikube ip):30080/deadlock

# 验证服务已不健康
curl http://$(minikube ip):30080/health
# 应该返回 500 + {"status":"unhealthy"}
```

### 步骤 5：观察 K8s 自动恢复

```bash
# 持续观察 Pod 状态
watch kubectl get pods -l app=game-service

# 约 15 秒后（periodSeconds=5 * failureThreshold=3），
# K8s 会自动重启容器
# 观察 RESTARTS 列从 0 变为 1
```

### 步骤 6：验证恢复

```bash
# 服务已自动恢复
curl http://$(minikube ip):30080/health
# 应该返回 {"status":"ok"}

# 查看 Pod 事件
kubectl describe pod -l app=game-service | grep -A5 "Events"
```

## 预期结果

### 时间线

| 时间 | 事件 | Pod 状态 |
|------|------|---------|
| 0s   | 部署完成 | Running, RESTARTS: 0 |
| 5s   | 发送 /deadlock 指令 | Running, RESTARTS: 0 |
| 10s  | Liveness Probe 检测到 /health 返回 500 | Running, RESTARTS: 0 |
| 15s  | 连续 3 次探测失败 | Running, RESTARTS: 0 |
| 20s  | K8s 重启容器 | Running, RESTARTS: 1 |
| 25s  | 新容器启动，/health 恢复正常 | Running, RESTARTS: 1 |

### 关键观察

```
$ kubectl get pods -l app=game-service
NAME                            READY   STATUS    RESTARTS   AGE
game-service-7d8b6c5f4-abc12   1/1     Running   0          30s

# 发送死锁后 ~15 秒
$ kubectl get pods -l app=game-service
NAME                            READY   STATUS    RESTARTS   AGE
game-service-7d8b6c5f4-abc12   1/1     Running   1          45s
```

## Liveness Probe 配置详解

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 3    # 容器启动后等待 3 秒再开始探测
  periodSeconds: 5           # 每 5 秒探测一次
  failureThreshold: 3        # 连续 3 次失败才判定为不健康
  timeoutSeconds: 2          # 每次探测超时时间为 2 秒
```

**判定时间**：`failureThreshold × periodSeconds = 3 × 5 = 15 秒`

## 关键概念

- **Liveness Probe（存活探针）**：检测容器是否还在正常运行
- **Readiness Probe（就绪探针）**：检测容器是否准备好接收流量
- **failureThreshold**：连续失败多少次才判定为不健康
- **自愈（Self-Healing）**：K8s 自动检测并修复故障容器
- **优雅关闭（Graceful Shutdown）**：`preStop` 钩子 + `terminationGracePeriodSeconds`
