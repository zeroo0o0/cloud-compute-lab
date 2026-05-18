# exp11 阿里云部署指南：探针检测与故障自愈

> 前置条件：已完成 [集群搭建](../cluster-setup/README.md)。

## 一、资源需求

| 项目 | 值 |
|------|-----|
| Namespace | `exp11` |
| Pod 数 | 1 |
| CPU | 50m request / 200m limit |
| 内存 | 32Mi request / 128Mi limit |
| 外部访问 | NodePort 30081 |

> 注意：exp11 的 NodePort 用 30081，避免与 exp9 的 30080 冲突。

## 二、构建并推送镜像

```bash
cd CourseCode/ch6/exp11

# 构建镜像
docker build -t ch6-exp11-game:latest .

# 推送 ACR
docker tag ch6-exp11-game:latest crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp11-game:latest
docker push crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp11-game:latest
```

## 三、修改 K8s 配置

### 3.1 game-deployment.yaml

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: game-service
  namespace: exp11
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
      terminationGracePeriodSeconds: 30
      containers:
      - name: game
        image: crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp11-game:latest
        imagePullPolicy: Always
        ports:
        - containerPort: 8080
        env:
        - name: PORT
          value: "8080"
        resources:
          requests:
            cpu: "50m"
            memory: "32Mi"
          limits:
            cpu: "200m"
            memory: "128Mi"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 3
          periodSeconds: 5
          failureThreshold: 3
          timeoutSeconds: 2
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 2
          periodSeconds: 5
          failureThreshold: 2
```

### 3.2 game-service.yaml

```yaml
apiVersion: v1
kind: Service
metadata:
  name: game-service
  namespace: exp11
spec:
  type: NodePort
  selector:
    app: game-service
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30081
```

## 四、部署

```bash
kubectl create namespace exp11
kubectl apply -f k8s/game-deployment.yaml
kubectl apply -f k8s/game-service.yaml

# 等待就绪
kubectl -n exp11 wait --for=condition=ready pod -l app=game-service --timeout=60s

# 验证
kubectl -n exp11 get pods
# NAME                            READY   STATUS    RESTARTS   AGE
# game-service-7d8b6c5f4-abc12   1/1     Running   0          10s
```

## 五、验证实验

### 5.1 获取访问地址

```bash
export NODE_IP=<任意节点公网IP>
```

### 5.2 正常状态验证

```bash
# 健康检查
curl http://$NODE_IP:30081/health
# {"healthy":true,"pod":"xxx","status":"ok"}

# 服务状态
curl http://$NODE_IP:30081/status
# {"deadlock_mode":false,"healthy":true,"pod_name":"xxx","requests":1,"status":"running","uptime":"30s"}

# 游戏接口
curl http://$NODE_IP:30081/game?player=test\&action=move
# {"action":"move","player":"test","pod":"xxx","result":"ok","time":"..."}

# 确认 RESTARTS = 0
kubectl -n exp11 get pods
# NAME                            READY   STATUS    RESTARTS   AGE
# game-service-7d8b6c5f4-abc12   1/1     Running   0          30s
```

### 5.3 模拟进程假死

```bash
# 发送死锁指令
curl -X POST http://$NODE_IP:30081/deadlock
# {"message":"Process is now simulating a deadlock...","status":"deadlock_activated"}

# 验证服务已不健康
curl http://$NODE_IP:30081/health
# 返回 500 + {"reason":"process deadlocked","status":"unhealthy"}

# Pod 状态暂时还是 Running（还没到探测阈值）
kubectl -n exp11 get pods
# NAME                            READY   STATUS    RESTARTS   AGE
# game-service-7d8b6c5f4-abc12   1/1     Running   0          45s
```

### 5.4 观察 K8s 自动恢复

```bash
# 持续观察 Pod 状态
watch -n 1 kubectl -n exp11 get pods

# 约 15 秒后（periodSeconds=5 × failureThreshold=3）：
# NAME                            READY   STATUS    RESTARTS   AGE
# game-service-7d8b6c5f4-abc12   1/1     Running   1          60s
#                                                    ^ RESTARTS 变为 1

# 查看 Pod 事件
kubectl -n exp11 describe pod -l app=game-service | grep -A10 "Events"
# Events:
#   Warning  Unhealthy  ...  Liveness probe failed: HTTP probe ... status code: 500
#   Normal   Killing    ...  Container game failed liveness probe, will be restarted
```

### 5.5 验证恢复

```bash
# 服务已自动恢复
curl http://$NODE_IP:30081/health
# {"healthy":true,"pod":"xxx","status":"ok"}

# RESTARTS = 1，状态 Running
kubectl -n exp11 get pods
# NAME                            READY   STATUS    RESTARTS   AGE
# game-service-7d8b6c5f4-abc12   1/1     Running   1          90s
```

## 六、预期结果时间线

```
时间    事件                                    RESTARTS
0s      部署完成，Pod Running                     0
5s      发送 /deadlock 指令                       0
10s     Liveness Probe 检测到 /health 返回 500    0
15s     连续 3 次探测失败                          0
20s     K8s 杀死容器并重启                         1
25s     新容器启动，/health 恢复正常               1
```

## 七、Liveness Probe 参数说明

```yaml
livenessProbe:
  httpGet:
    path: /health        # 探测路径
    port: 8080           # 探测端口
  initialDelaySeconds: 3 # 容器启动后等 3 秒再开始探测
  periodSeconds: 5       # 每 5 秒探测一次
  failureThreshold: 3    # 连续 3 次失败才判定不健康
  timeoutSeconds: 2      # 每次探测超时 2 秒
```

**判定不健康的时间** = `failureThreshold × periodSeconds` = 3 × 5 = **15 秒**

## 八、清理

```bash
kubectl delete namespace exp11
```
