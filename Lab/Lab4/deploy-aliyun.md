# Lab4 阿里云部署指南：云原生部署 Docker + Kubernetes

> 前置条件：已完成 [集群搭建](../../CourseCode/ch6/cluster-setup/README.md)。
> Lab4 需要的资源最多（4 个 Pod），建议单独部署，或在 exp9/10/11 测试完毕后清理再部署。

## 一、资源需求

| 项目 | 值 |
|------|-----|
| Namespace | `Lab4` |
| Pod 数 | Phase1: 1 / Phase2: 4 |
| CPU | Phase2: 400m request / 2000m limit |
| 内存 | Phase2: 256Mi request / 1024Mi limit |
| 存储 | PVC 1Gi（使用云盘 StorageClass） |
| 外部访问 | NodePort 30310 |

### 资源占用

```
Phase 1（单体模式）:
  1 Pod:  100m CPU / 64Mi RAM

Phase 2（微服务模式）:
  1 Gateway Pod:  100m CPU / 64Mi RAM
  3 Node Pod:     300m CPU / 192Mi RAM (每 Pod 100m/64Mi)
  合计:           400m CPU / 256Mi RAM
```

> 3 台 2C8G 节点（总 6 CPU / 24GiB RAM）完全可以承载。

## 二、构建并推送镜像

```bash
cd Lab/Lab4/student

# 构建镜像
docker build -t battleworld:latest .

# 推送 ACR
docker tag battleworld:latest crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/battleworld:latest
docker push crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/battleworld:latest
```

## 三、修改 K8s 配置

### 3.1 修改所有 YAML 中的镜像地址

需要修改以下文件中的 `image` 和 `imagePullPolicy`：

**k8s/deployment.yaml**（Phase 1）：

```yaml
spec:
  containers:
  - name: battleworld
    image: crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/battleworld:latest
    imagePullPolicy: Always
```

**k8s/gateway-deployment.yaml**（Phase 2 Gateway）：

```yaml
spec:
  containers:
  - name: gateway
    image: crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/battleworld:latest
    imagePullPolicy: Always
    command: ["gateway"]
```

**k8s/node-statefulset.yaml**（Phase 2 Nodes）：

```yaml
spec:
  containers:
  - name: node
    image: crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/battleworld:latest
    imagePullPolicy: Always
    command: ["node"]
```

### 3.2 修改 PVC 使用云盘 StorageClass

**k8s/pvc.yaml**：

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: battleworld-data
  namespace: Lab4
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: alicloud-disk-essd   # ACK 默认 StorageClass
  resources:
    requests:
      storage: 20Gi   # 阿里云云盘最小 20Gi
```

> 如果使用 kubeadm 自建集群（非 ACK），需要手动安装 CSI 插件。
> 或者使用 `hostPath` 替代 PVC（适合实验环境）：

```yaml
# 替代方案：hostPath（实验用）
apiVersion: v1
kind: PersistentVolume
metadata:
  name: battleworld-pv
spec:
  capacity:
    storage: 1Gi
  accessModes:
    - ReadWriteOnce
  hostPath:
    path: /data/battleworld
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: battleworld-data
  namespace: Lab4
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

### 3.3 快速批量替换脚本

```bash
cd Lab/Lab4

# 替换镜像地址
ACR="crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute"
sed -i "s|image: battleworld:latest|image: ${ACR}/battleworld:latest|g" k8s/*.yaml
sed -i "s|imagePullPolicy: Never|imagePullPolicy: Always|g" k8s/*.yaml
```

## 四、部署 Phase 1（单体容器化）

```bash
cd Lab/Lab4

# 创建 Namespace
kubectl apply -f k8s/namespace.yaml

# 部署 ConfigMap、PVC、Deployment、Service
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/pvc.yaml
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml

# 等待就绪
kubectl -n Lab4 wait --for=condition=ready pod -l app=battleworld --timeout=120s

# 验证
kubectl -n Lab4 get pods
kubectl -n Lab4 get svc
```

### 验证 Phase 1

```bash
export NODE_IP=<任意节点公网IP>

# 连接游戏
cd Lab/Lab4/student
go run ./cmd/client
# 选择 2. 连接指定网关
# IP: $NODE_IP
# 端口: 30310

# 或使用 admin 工具
go run ./cmd/admin 状态 $NODE_IP:30310
```

## 五、部署 Phase 2（微服务拆分）

> Phase 2 会替换 Phase 1 的 Deployment，两者共享同一个 Namespace。

```bash
cd Lab/Lab4

# 删除 Phase 1 的 Deployment（保留 Namespace、ConfigMap、PVC）
kubectl -n Lab4 delete deployment battleworld

# 应用 Phase 2 的 ConfigMap（会更新 gateway-config 和 node-config）
kubectl apply -f k8s/configmap.yaml

# 部署 Gateway
kubectl apply -f k8s/gateway-deployment.yaml

# 部署 Node StatefulSet
kubectl apply -f k8s/node-statefulset.yaml
kubectl apply -f k8s/headless-service.yaml

# 等待所有 Pod 就绪
kubectl -n Lab4 wait --for=condition=ready pod -l app=battleworld-gateway --timeout=120s
kubectl -n Lab4 wait --for=condition=ready pod -l app=battleworld-node --timeout=120s

# 验证
kubectl -n Lab4 get pods
# NAME                                READY   STATUS    RESTARTS   AGE
# battleworld-gateway-7d8b6c5f4-xxx  1/1     Running   0          30s
# battleworld-node-0                 1/1     Running   0          25s
# battleworld-node-1                 1/1     Running   0          20s
# battleworld-node-2                 1/1     Running   0          15s
```

### 验证 Phase 2

```bash
export NODE_IP=<任意节点公网IP>

# 查看 Service
kubectl -n Lab4 get svc
# NAME                  TYPE       CLUSTER-IP     EXTERNAL-IP   PORT(S)          AGE
# battleworld-gateway   NodePort   10.96.xxx.xxx  <none>        9310:30310/TCP   1m
# battleworld-nodes     ClusterIP  None           <none>        9311/TCP         1m

# 查看 StatefulSet DNS
kubectl -n Lab4 get pods -o wide -l app=battleworld-node
# NAME                READY  STATUS   NODE      ...
# battleworld-node-0  1/1    Running  worker1
# battleworld-node-1  1/1    Running  worker2
# battleworld-node-2  1/1    Running  worker1

# 连接游戏
cd Lab/Lab4/student
go run ./cmd/client
# 选择 2. 连接指定网关
# IP: $NODE_IP
# 端口: 30310

# 管理命令
go run ./cmd/admin 状态 $NODE_IP:30310
```

### 验证服务发现

```bash
# 进入 Gateway Pod
kubectl -n Lab4 exec -it deployment/battleworld-gateway -- sh

# 测试 DNS 解析
nslookup battleworld-node-0.battleworld-nodes
nslookup battleworld-node-1.battleworld-nodes
nslookup battleworld-node-2.battleworld-nodes

# 测试节点连通性
wget -qO- http://battleworld-node-0.battleworld-nodes:9311/
wget -qO- http://battleworld-node-1.battleworld-nodes:9311/
wget -qO- http://battleworld-node-2.battleworld-nodes:9311/
```

## 六、使用部署脚本

Lab4 自带的 `deploy.sh` 需要适配阿里云环境。以下为修改后的版本：

```bash
#!/bin/bash
set -e

ACR="crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute"
NAMESPACE="Lab4"

echo "=== 1. 构建镜像 ==="
cd student
docker build -t ${ACR}/battleworld:latest .
docker push ${ACR}/battleworld:latest
cd ..

echo "=== 2. 替换镜像地址 ==="
sed -i "s|image:.*battleworld.*|image: ${ACR}/battleworld:latest|g" k8s/*.yaml
sed -i "s|imagePullPolicy: Never|imagePullPolicy: Always|g" k8s/*.yaml

echo "=== 3. 部署 K8s 资源 ==="
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/pvc.yaml
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml

echo "=== 4. 等待就绪 ==="
kubectl -n ${NAMESPACE} wait --for=condition=ready pod -l app=battleworld --timeout=120s

echo "=== 5. 获取访问地址 ==="
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="ExternalIP")].address}')
if [ -z "$NODE_IP" ]; then
  NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
fi

echo ""
echo "=== 部署完成 ==="
echo "访问地址: ${NODE_IP}:30310"
echo ""
echo "连接游戏: go run ./cmd/client → 选择 2 → IP: ${NODE_IP} → 端口: 30310"
echo "管理命令: go run ./cmd/admin 状态 ${NODE_IP}:30310"
```

## 七、调试

```bash
# 查看所有资源
kubectl -n Lab4 get all

# 查看 Pod 日志
kubectl -n Lab4 logs -l app=battleworld-gateway -f
kubectl -n Lab4 logs -l app=battleworld-node -f

# 进入 Pod 调试
kubectl -n Lab4 exec -it deployment/battleworld-gateway -- sh
kubectl -n Lab4 exec -it battleworld-node-0 -- sh

# 查看 PVC 状态
kubectl -n Lab4 get pvc
kubectl -n Lab4 describe pvc battleworld-data

# 查看 ConfigMap
kubectl -n Lab4 get configmap
kubectl -n Lab4 describe configmap battleworld-config

# 查看事件
kubectl -n Lab4 get events --sort-by=.lastTimestamp
```

## 八、清理

```bash
# 删除整个 Namespace（包含所有资源）
kubectl delete namespace Lab4

# 如果使用了 hostPath PV，也需要删除
kubectl delete pv battleworld-pv
```
