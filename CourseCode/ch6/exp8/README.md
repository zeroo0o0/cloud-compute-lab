# 实验八：无状态网关与会话重连

本实验演示：**网关只负责连接与路由，玩家会话全部交给 Redis，游戏状态交给 Storage**。  
因此，即使当前连接所在的 `Gateway-Pod` 被删除，外部客户端也只会短暂断线，随后携带同一个 token 自动连到另一个网关，并从 Redis 中恢复原来的 session。

课堂上要让学生看到三件事：

- `gateway` 有 2 个副本，外部入口通过 Service 转发到任意一个网关 Pod。
- 玩家 session 存在 Redis 中，不存在某一个 Gateway-Pod 的内存里。
- 删除当前连接所在的 Gateway-Pod 后，客户端会自动重连，并看到 `resumed=true`。

## 目录结构

```text
exp8/
├── README.md                    # 本实验说明文档
├── go.work                      # 允许在 exp8 目录直接运行 Go 客户端
├── gateway-session.yaml         # ConfigMap + Redis + 2 副本网关 + ClusterIP + NodePort
├── build-images.sh              # 构建、标记并推送镜像
├── Dockerfile                   # 直接从 Go 源码构建镜像
├── Dockerfile.prebuilt          # 使用预编译二进制构建镜像
├── dist/                        # build-images.sh 生成的 gateway/game/storage 二进制
└── game-app/                    # gateway、game、storage、client 源码
```

## 0. 登录云上 Kubernetes 集群

先 SSH 登录可以操作集群的服务器，例如节点 A：

```bash
ssh <用户名>@<节点地址>
```

进入本实验目录：

```bash
cd /path/to/ch6/exp8
```

确认当前 `kubectl` 已经连到云上多节点集群：

```bash
kubectl get nodes -o wide
```

预期能看到 4 个节点处于 `Ready`，例如：

```text
NAME    STATUS   ROLES           INTERNAL-IP
k8s-a   Ready    control-plane   ...
k8s-b   Ready    <none>          ...
k8s-c   Ready    <none>          ...
k8s-d   Ready    <none>          10.0.2.12
```

## 1. 构建并推送镜像

本实验统一使用节点 D 上的本地镜像仓库：

```text
10.0.2.12:5000
```

最终 Kubernetes YAML 使用的镜像是：

```text
10.0.2.12:5000/exp8/exp8-gateway:v1
10.0.2.12:5000/exp8/exp8-game:v1
10.0.2.12:5000/exp8/exp8-storage:v1
10.0.2.12:5000/exp8/redis:v1
```

在 `exp8` 目录执行：

```bash
bash build-images.sh
```

脚本会直接使用当前 `exp8` 目录作为 Docker 构建上下文，并依次完成：

```bash
for service in gateway game storage; do
  docker build -f Dockerfile.prebuilt --build-arg SERVICE="$service" -t "exp8-$service:v1" .
  docker tag "exp8-$service:v1" "10.0.2.12:5000/exp8/exp8-$service:v1"
  docker push "10.0.2.12:5000/exp8/exp8-$service:v1"
done

docker pull redis:7-alpine
docker tag redis:7-alpine 10.0.2.12:5000/exp8/redis:v1
docker push 10.0.2.12:5000/exp8/redis:v1
```

确认本地 Docker 镜像：

```bash
docker images | grep exp8
docker images | grep redis
```

## 2. 部署 Storage、Game、Redis 和 2 个网关副本

应用 YAML：

```bash
kubectl apply -f gateway-session.yaml
```

查看 Pod 和 Service：

```bash
kubectl get pods -o wide
kubectl get svc storage-service game-service redis-service gateway-service gateway-service-external
```

预期能看到：

```text
storage-xxxxx    1/1   Running
game-xxxxx       1/1   Running
redis-xxxxx      1/1   Running
gateway-xxxxx    1/1   Running
gateway-yyyyy    1/1   Running
```

`gateway-session.yaml` 里有三个关键点：

```yaml
replicas: 2

- name: REDIS_ADDR
  valueFrom:
    configMapKeyRef:
      name: gateway-config
      key: redis_addr

gateway-service-external:
  type: NodePort
  nodePort: 30088
```

这对应 PPT 里的三件事：
- 网关配置从统一配置源注入。
- `gateway` 有 2 个副本，客户端连接可能落到任意一个 Gateway-Pod。
- 网关自身不保存会话，Redis 才是状态源。
- 外部玩家通过稳定入口访问网关。


## 3. 从集群外部打开客户端连接

先查看节点 IP：

```bash
kubectl get nodes -o wide
```

选择任意一个可以从当前终端访问的节点 IP，例如节点 A 的公网 IP 或内网 IP。`gateway-service-external` 暴露的 NodePort 是 `30088`，所以网关地址格式是：

```text
<Node-IP>:30088
```

例如：

```text
120.79.8.174:30088
10.0.2.10:30088
```

在 `exp8` 目录直接运行可交互游戏客户端。`exp8/go.work` 已经指向 `game-app` 模块，所以不需要再 `cd game-app`：

```bash
GATEWAY_ADDR="<Node-IP>:30088" go run ./game-app/cmd/client
```

客户端会先发送 `HELLO <token> <playerID>` 建立 session，然后每轮用 `GET` 查看位置、用 `MOVE` 移动玩家。你会看到类似：

```text
WELCOME gateway=gateway-abcde resumed=false player=student-1 game=http://game-service:8081 heartbeats=0
当前位置: x=0,y=0
请输入方向 (w/a/s/d)、h 心跳、q 退出:
```

如果在云服务器 `k8s-a` 上本机验证，推荐优先使用节点内网 IP。如果内网 IP 能通、公网 IP 超时，通常是云服务器安全组或防火墙还没有放行 TCP `30088`。

## 4. 观察 Redis 里确实有会话

进入 Redis：

```bash
kubectl exec -it deploy/redis -- redis-cli
```

在 Redis 里查看 session：

```redis
HGETALL session:student-1-token
TTL session:student-1-token
```

你会看到玩家 ID、game server、heartbeat 计数和 TTL。  
这一步是实验的脊梁：**状态在 Redis，不在某一个 Gateway-Pod 的内存里。**

玩家坐标则在 `storage` 服务里。客户端移动后，可以通过客户端界面看到坐标变化；链路是：

```text
client GET/MOVE -> gateway -> game HTTP -> storage HTTP
```

## 5. 模拟网关突然崩溃

先看客户端当前连的是哪个网关。客户端日志里会显示：

```text
gateway=gateway-abcde
```

找到同名 Pod 后，强制删除它：

```bash
kubectl delete pod gateway-abcde --force --grace-period=0
```

只要外部入口还在，客户端就会先看到一次断线，然后自动重连：

```text
[client] disconnected: ...
[client] connecting addr=... token=student-1-token
WELCOME gateway=gateway-fghij resumed=true player=student-1 game=http://game-service:8081 heartbeats=...
```

关键观察点：

```text
旧网关没了
token 没变
新网关变了
heartbeat_count 没清零
```

这就是“无感飘移”：连接换了接班人，session 没丢。


## 6. 为什么这说明网关是无状态的

```text
外部 client
      ↓
NodePort 30088
      ↓
任意 Gateway-Pod
      ↓
Redis session
```

网关 Pod 只做两件事：

- 接受长连接
- 根据 token 去 Redis 读写 session

所以只要 Redis 还在，任意新的 Gateway-Pod 都能接手同一个玩家。

## 7. 课堂展示脚本

1. 展示 `gateway-session.yaml`：
   - `redis`
   - `gateway replicas: 2`
   - `REDIS_ADDR` 来自 `ConfigMap`
   - `gateway-service-external`
   - `nodePort: 30088`

2. 执行：

```bash
kubectl apply -f gateway-session.yaml
kubectl get pods -o wide
kubectl get svc gateway-service gateway-service-external
```

3. 启动外部客户端：

```bash
GATEWAY_ADDR="<NodeIP>:30088" go run ./game-app/cmd/client
```

4. 用客户端实际移动一次，确认 `GET/MOVE` 能走完整游戏链路。

5. 进 Redis 看 session：

```bash
kubectl exec -it deploy/redis -- redis-cli
HGETALL session:student-1-token
```

6. 删除当前承载连接的 gateway Pod：

```bash
kubectl delete pod <当前网关Pod名> --force --grace-period=0
```

6. 观察客户端自动重连，并看到：

```text
resumed=true
gateway=<新的 Pod 名>
heartbeats=<继续累加>
```

## 8. 清理

```bash
kubectl delete -f gateway-session.yaml
```

确认资源已经删除：

```bash
kubectl get pods
kubectl get svc
```
