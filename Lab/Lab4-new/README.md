# Lab4：自建 Kubernetes 部署说明

这份目录用于把 Lab4 分布式游戏部署到自建 Kubernetes 集群，并验证 HPA 自动扩缩容。

默认结果：

```text
命名空间：lab4
公网入口：<任意 Node 公网 IP>:30910
组件：gateway / coordinator / map-green / map-cave / map-ruins / redis
HPA：5 个业务 Deployment 都开启，min=1，max=10
状态：Redis 保存用户、热会话、地图 checkpoint、leader 租约
回收保护：Pod drain + deletion-cost + 客户端自动重连
```

## 1. 推荐部署流程

以下命令以这套 4 台机器为例：

```text
k8s-a 公网：120.79.8.174，control-plane
k8s-b 内网：10.0.2.10，worker
k8s-c 内网：10.0.2.11，worker
k8s-d 内网：10.0.2.12，worker
```

worker 通常只有内网 IP，本机不能直接访问，所以推荐把目录传到主节点 `k8s-a`，在主节点构建、导入镜像、部署。

如果主节点已有旧目录，先备份：

```bash
ssh root@120.79.8.174
[ -d /root/Lab4 ] && mv /root/Lab4 /root/Lab4.bak.$(date +%Y%m%d%H%M%S)
exit
```

本机上传：

```bash
cd Lab
scp -r Lab4-new root@120.79.8.174:/root/Lab4
ssh root@120.79.8.174
```

主节点执行：

```bash
cd /root/Lab4

go test ./...

./scripts/build-images.sh v1

./scripts/import-images-to-nodes.sh \
  .dist/images/lab4-images-v1.tar \
  10.0.2.10 10.0.2.11 10.0.2.12

./scripts/deploy-k8s.sh \
  lab4/gateway:v1 \
  lab4/coordinator:v1 \
  lab4/map:v1 \
  redis:7-alpine
```

如果你允许 control-plane 也跑业务 Pod，也要在 `k8s-a` 本机导入镜像：

```bash
ctr -n k8s.io images import .dist/images/lab4-images-v1.tar
```

## 2. 前置条件

集群需要已经可用：

```bash
kubectl get nodes -o wide
kubectl get pods -A
```

所有节点应为 `Ready`。

HPA 需要 metrics-server：

```bash
kubectl top nodes
kubectl top pods -A
```

如果 `kubectl top` 不可用，先安装 metrics-server：

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/download/v0.8.1/components.yaml

kubectl -n kube-system set image deployment/metrics-server \
  metrics-server=registry.aliyuncs.com/google_containers/metrics-server:v0.8.1

kubectl -n kube-system patch deployment metrics-server --type=json \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'

kubectl -n kube-system rollout status deployment/metrics-server --timeout=180s
kubectl top nodes
```

## 3. 部署后检查

查看 Pod、HPA、Service：

```bash
kubectl -n lab4 get pods,hpa -o wide
kubectl -n lab4 get svc
kubectl -n lab4 get statefulset
```

正常应看到：

```text
lab4-gateway
lab4-coordinator
lab4-map-green
lab4-map-cave
lab4-map-ruins
lab4-redis-0
```

查看公网入口：

```bash
kubectl -n lab4 get svc lab4-gateway
```

应看到：

```text
9310:30910/TCP
```

验证游戏服务：

```bash
go run ./cmd/admin 状态 120.79.8.174:30910
```

启动客户端：

```bash
go run ./cmd/client 120.79.8.174:30910
```

查看 Redis 数据：

```bash
kubectl -n lab4 exec lab4-redis-0 -- redis-cli KEYS 'lab4*'
```

运行过游戏后通常会看到：

```text
lab4:user:<用户名>
lab4:session:<用户名>
lab4:checkpoint:green
lab4:checkpoint:cave
lab4:checkpoint:ruins
lab4-leader-coordinator
lab4-leader-map-green
lab4-leader-map-cave
lab4-leader-map-ruins
```

## 4. HPA 压测

另开一个终端观察：

```bash
watch -n 0.5 'kubectl -n lab4 get pods,hpa -o wide'
```

开始压测：

```bash
ADDR=120.79.8.174:30910 ./scripts/load-test-hpa.sh
```

加大压力：

```bash
ADDR=120.79.8.174:30910 CLIENTS=600 OPS_PER_CLIENT=8 DURATION=10m ./scripts/load-test-hpa.sh
```

判断 HPA 生效：

```text
HPA TARGETS 接近或超过 60%
REPLICAS 从 1 增加到更多
新增 Pod 进入 Running
```

压测结束后不会立刻缩容，HPA 默认有缩容稳定窗口，通常要等几分钟。

## 5. Pod 回收保护

这版希望缩容时尽量不回收正在承载玩家的 Pod。实现方式：

```text
gateway：统计 TCP 玩家连接数，drain 后不再接新连接
coordinator：统计在线 session，drain 后停止成为 leader
map：统计地图玩家数，drain 前保存 Redis checkpoint
所有业务 Pod：上报 controller.kubernetes.io/pod-deletion-cost
客户端：断线后自动重连并恢复原会话
```

查看每个 Pod 的活跃玩家数和删除成本：

```bash
kubectl -n lab4 get pods \
  -o custom-columns=NAME:.metadata.name,ACTIVE:.metadata.annotations.lab4/active-players,DRAINING:.metadata.annotations.lab4/draining,COST:.metadata.annotations.controller\\.kubernetes\\.io/pod-deletion-cost,NODE:.spec.nodeName
```

边界说明：Kubernetes/HPA 原生不能 100% 保证“绝不删除有玩家的 Pod”。本实验通过 drain、deletion-cost、Redis 恢复和客户端自动重连，尽量让用户只感受到短暂卡顿。

## 6. 常见问题

### 6.1 Pod 是 ImagePullBackOff

通常是节点没有对应镜像。重新导入到所有 worker：

```bash
./scripts/import-images-to-nodes.sh \
  .dist/images/lab4-images-v1.tar \
  10.0.2.10 10.0.2.11 10.0.2.12
```

确认节点里有镜像：

```bash
ctr -n k8s.io images ls | grep -E 'lab4|redis'
```

### 6.2 HPA 是 `<unknown>`

检查 metrics-server：

```bash
kubectl -n kube-system get pods | grep metrics-server
kubectl top nodes
kubectl top pods -n lab4
```

### 6.3 访问 `120.79.8.174:30910` 不通

检查：

```bash
kubectl -n lab4 get svc lab4-gateway
kubectl -n lab4 get pods -l app=lab4-gateway -o wide
```

同时确认云服务器安全组放行：

```text
TCP 30910
```

### 6.4 旧 Pod 一直 Terminating

新版有 drain 和较长 `terminationGracePeriodSeconds`，短时间 Terminating 正常。超过 5-6 分钟再看：

```bash
kubectl -n lab4 describe pod <pod-name>
```

### 6.5 Redis 数据在哪里

默认 Redis 数据在运行 `lab4-redis-0` 的节点：

```text
/var/lib/lab4/redis
```

查看 Redis 跑在哪台机器：

```bash
kubectl -n lab4 get pod lab4-redis-0 -o wide
```

## 7. 常用修改

修改 NodePort：

```text
deploy/k8s/gateway-service.yaml
```

修改 HPA 范围：

```text
deploy/k8s/hpa.yaml
```

修改资源 requests/limits：

```text
deploy/k8s/*-deployment.yaml
```

## 8. 清理

删除整个实验：

```bash
kubectl delete namespace lab4
```

如果只是重新部署新镜像，不要删除 namespace，重新执行：

```bash
./scripts/deploy-k8s.sh \
  lab4/gateway:v1 \
  lab4/coordinator:v1 \
  lab4/map:v1 \
  redis:7-alpine
```

## 9. Windows 说明

Windows 推荐使用 WSL2 Ubuntu，然后按上面的 Linux 命令执行。

如果不用 WSL，也可以使用 PowerShell 脚本：

```powershell
.\scripts\build-images.ps1 -Tag v1

.\scripts\import-images-to-nodes.ps1 `
  -ImageTar .dist\images\lab4-images-v1.tar `
  -Nodes 10.0.2.10,10.0.2.11,10.0.2.12

.\scripts\deploy-k8s.ps1 `
  -GatewayImage lab4/gateway:v1 `
  -CoordinatorImage lab4/coordinator:v1 `
  -MapImage lab4/map:v1 `
  -RedisImage redis:7-alpine
```
