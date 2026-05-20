# 实验五：多服编排与服务发现

本实验使用 `game-app` 中的三个微服务：

- `gateway`：TCP 文本协议入口，接收 `GET/MOVE` 命令。
- `game`：HTTP 战斗逻辑服务，不保存状态。
- `storage`：HTTP 状态存储服务。

课堂上要让学生看到两件事：

- 一条 `kubectl apply -f game-cluster.yaml` 同时创建 3 个 Deployment 和 3 个 Service。
- `gateway` 不知道 `game` Pod 的 IP，只通过 `http://game-service:8081` 这个服务名访问下游；`game` 同理通过 `http://storage-service:8082` 访问存储。

## 目录结构

```text
exp5/
├── README.md                    # 本实验说明文档
├── go.work                      # 允许在 exp5 目录直接运行 Go 客户端
├── game-cluster.yaml            # 一次创建 3 个 Deployment 和 3 个 Service
├── build-images.sh              # 构建、标记并推送镜像
├── Dockerfile                   # 直接从 Go 源码构建镜像
├── Dockerfile.prebuilt          # 使用预编译二进制构建极简镜像
├── dist/                        # build-images.sh 生成的 Linux 二进制
└── game-app/                    # 待部署的游戏应用源码
```

## 0. 登录云上 Kubernetes 集群

先 SSH 登录可以操作集群的服务器，例如节点 A：

```bash
ssh <用户名>@<节点地址>
```

进入本实验目录：

```bash
cd /path/to/ch6/exp5
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
10.0.2.12:5000/exp5/exp5-gateway:v1
10.0.2.12:5000/exp5/exp5-game:v1
10.0.2.12:5000/exp5/exp5-storage:v1
```


在 `exp5` 目录执行：

```bash
bash build-images.sh
```

脚本会直接使用当前 `exp5` 目录作为 Docker 构建上下文，并依次完成：

```bash
docker build -f Dockerfile.prebuilt --build-arg SERVICE=gateway -t exp5-gateway:v1 .
docker build -f Dockerfile.prebuilt --build-arg SERVICE=game -t exp5-game:v1 .
docker build -f Dockerfile.prebuilt --build-arg SERVICE=storage -t exp5-storage:v1 .

docker tag exp5-gateway:v1 10.0.2.12:5000/exp5/exp5-gateway:v1
docker tag exp5-game:v1 10.0.2.12:5000/exp5/exp5-game:v1
docker tag exp5-storage:v1 10.0.2.12:5000/exp5/exp5-storage:v1

docker push 10.0.2.12:5000/exp5/exp5-gateway:v1
docker push 10.0.2.12:5000/exp5/exp5-game:v1
docker push 10.0.2.12:5000/exp5/exp5-storage:v1
```

```

确认本地 Docker 镜像：

```bash
docker images | grep exp5
```

## 2. 部署到 Kubernetes

应用 YAML：

```bash
kubectl apply -f game-cluster.yaml
```

查看 Pod 和 Service：

```bash
kubectl get pods -o wide
kubectl get svc storage-service game-service gateway-service
```

预期能看到：

```text
storage-xxxxx   1/1   Running
game-xxxxx      1/1   Running
gateway-xxxxx   1/1   Running
```

`gateway-service` 是 `NodePort`，固定暴露到每个节点的 `30085` 端口：

```bash
kubectl get svc gateway-service
```

## 3. 演示服务发现

先查看当前 Pod IP 和 Service：

```bash
kubectl get pods -o wide
kubectl get svc storage-service game-service gateway-service
```

删除一个 `game` Pod，让 Deployment 自动拉起新 Pod：

```bash
kubectl delete pod -l app=game
kubectl get pods -o wide
```

再看服务名仍然稳定存在：

```bash
kubectl get svc storage-service game-service gateway-service
```

Pod 会被重建，IP 可能变化；Service 名称不变，调用方因此不需要关心后端 Pod 的具体 IP。

关键配置在 `game-cluster.yaml`：

```yaml
- name: GAME_URL
  value: "http://game-service:8081"

- name: STORAGE_URL
  value: "http://storage-service:8082"
```

这就是服务发现：调用方使用稳定的 Service DNS 名称，不关心后端 Pod IP。

## 4. 通过网关发起战斗请求


先查看节点 IP：

```bash
kubectl get nodes -o wide
```

选择任意一个可以从当前终端访问的节点 IP，例如节点 A 的公网 IP 或内网 IP，然后访问 `30085`：

```bash
printf "GET student-1\n" | nc <Node-IP> 30085
printf "MOVE student-1 d\n" | nc <Node-IP> 30085
printf "GET student-1\n" | nc <Node-IP> 30085
```
120.79.8.174:30085
10.0.2.10:30085
成功输出示例：

```text
RESULT ok position=(x=0,y=0)
RESULT ok position=(x=1,y=0)
```

如果使用 Go 客户端：

```bash
CLIENT_SERVER_URL="120.79.8.174:30085" go run ./game-app/cmd/client
```
成功输出示例：
```text
网关地址 (默认 127.0.0.1:8080): 120.79.8.174:30085
客户端ID: client-295263
当前位置: x=0,y=0
+----------------------+
|P . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
+----------------------+
```


## 5. 课堂展示脚本

1. 部署：

```bash
kubectl apply -f game-cluster.yaml
kubectl get pods -w
```

2. 展示链路：

```text
client -> gateway-service -> Gateway-Pod -> game-service -> Game-Pod -> storage-service -> Storage-Pod
```

3. 删除 game Pod，观察 Deployment 自动拉起新 Pod：

```bash
kubectl delete pod -l app=game
kubectl get pods -w
```

4. 重启后继续请求，说明网关仍然访问 `game-service`，不需要知道新 Pod IP。

## 6. 清理

```bash
kubectl delete -f game-cluster.yaml
```

确认资源已经删除：

```bash
kubectl get pods
kubectl get svc
```
