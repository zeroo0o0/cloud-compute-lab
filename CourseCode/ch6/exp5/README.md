# 实验五：多服编排与服务发现

本实验使用 `game-app` 中的三个微服务（对应上一章exp1代码）：

- `gateway`：TCP 文本协议入口，接收 `GET/MOVE` 命令。
- `game`：HTTP 战斗逻辑服务，不保存状态。
- `storage`：HTTP 状态存储服务。

课堂上要让学生看到两件事：

- 一条 `kubectl apply -f game-cluster.yaml` 同时创建 3 个 Deployment 和 3 个 Service。
- `gateway` 不知道 `game` Pod 的 IP，只通过 `http://game-service:8081` 这个服务名访问下游；`game` 同理通过 `http://storage-service:8082` 访问存储。

## 目录结构（当前）

```text
exp5/
├── README.md                    # 本实验说明文档
├── game-cluster.yaml            # 一次创建 3 个 Deployment 和 3 个 Service
├── build-images.sh              # WSL / Ubuntu 下构建镜像脚本
├── Dockerfile                   # 直接从 Go 源码构建镜像
├── Dockerfile.prebuilt          # 使用预编译二进制构建极简镜像
├── dist/                        # build-images.sh 生成的 Linux 二进制
└── game-app/                    # 待部署的游戏应用源码

```

## 0. 准备exp5 Minikube 集群


先打开 Ubuntu / WSL：

```powershell
wsl -d Ubuntu
```

进入本项目目录。Windows 的 `C:\ch6` 在 WSL 里通常对应：

```bash
cd "/mnt/c/ch6"
```

新建并启动一个名为 `exp5` 的 Minikube 集群：

```bash
minikube start -p exp5
```

确认当前操作的是 `exp5` 集群：

```bash
kubectl config current-context
kubectl get nodes
```

看到当前上下文是 `exp5`，并且节点状态类似下面这样，就说明可以继续：

```text
exp5

NAME   STATUS   ROLES           AGE   VERSION
exp5   Ready    control-plane   ...
```

## 1. 构建镜像

### 方式 A：先在 WSL 编译，再用极简镜像打包（推荐）

在 Ubuntu / WSL 里执行 Bash 脚本。它会先把 Go 服务编译成 Linux 静态二进制，再使用 `Dockerfile.prebuilt` 和 `FROM scratch` 打镜像，不需要在构建镜像时拉取 Go 基础镜像。

构建镜像：

```bash
bash exp5/build-images.sh
```

### 方式 B：直接使用 Dockerfile 从源码构建

如果你不想在 WSL 本地安装 Go，也可以让 Docker 在构建镜像时完成编译。这个方式会使用 `Dockerfile`，但需要能够拉取 `golang:1.22-alpine` 和 `alpine` 等基础镜像。

在 `ch6` 目录执行：

```bash
docker build -f exp5/Dockerfile --build-arg TARGET=gateway -t exp5-gateway:v1 .
docker build -f exp5/Dockerfile --build-arg TARGET=game -t exp5-game:v1 .
docker build -f exp5/Dockerfile --build-arg TARGET=storage -t exp5-storage:v1 .
```

两种方式任选其一即可。完成后，把镜像加载进 Minikube：

```bash
minikube -p exp5 image load exp5-gateway:v1
minikube -p exp5 image load exp5-game:v1
minikube -p exp5 image load exp5-storage:v1
```


## 2. 一键编排

在 Ubuntu / WSL 里执行：

```bash
kubectl apply -f exp5/game-cluster.yaml
kubectl get pods -o wide
kubectl get svc
```

预期能看到：

```text
storage-xxxxx   1/1   Running
game-xxxxx      1/1   Running
gateway-xxxxx   1/1   Running
```

## 3. 演示服务发现

先看当前 Pod IP以及service：

```bash
kubectl get pods -o wide
kubectl get svc storage-service game-service gateway-service
```

可以删掉一个 `game` Pod，让 Deployment 自动拉起新的 Pod，再对比 IP：

```bash
kubectl delete pod -l app=game
kubectl get pods -o wide
```

再看服务名稳定存在：

```bash
kubectl get svc storage-service game-service gateway-service
```
Pod 会被重建，IP 可能变化；Service 名称不变，调用方因此不需要关心后端 Pod 的具体 IP。

关键在 `game-cluster.yaml`：

```yaml
- name: GAME_URL
  value: "http://game-service:8081"

- name: STORAGE_URL
  value: "http://storage-service:8082"
```

这就是服务发现：调用方使用稳定的 Service DNS 名称，不关心后端 Pod IP。

## 4. 通过网关发起战斗请求

方式 A：推荐使用端口转发，再启动实验一客户端。

先在 Ubuntu / WSL 里执行端口转发。这条命令会一直挂着，不要关这个终端：

```bash
kubectl port-forward svc/gateway-service 8080:8080
```

再打开另一个 Ubuntu / WSL 终端：

```bash
cd "/mnt/c/ch6/exp5/game-app"
CLIENT_SERVER_URL="127.0.0.1:8080" go run ./cmd/client
```

如果你希望在 Windows PowerShell 里运行客户端，可以这样：

```powershell
cd exp5/game-app
$env:CLIENT_SERVER_URL="127.0.0.1:8080"
go run ./cmd/client
```

方式 B：直接发 TCP 文本命令。需要你的环境里安装了 `ncat` 或 `nc`。

```bash
printf "GET student-1\n" | nc 127.0.0.1 8080
printf "MOVE student-1 d\n" | nc 127.0.0.1 8080
printf "GET student-1\n" | nc 127.0.0.1 8080
```

成功输出示例：

```text
RESULT ok position=(x=0,y=0)
RESULT ok position=(x=1,y=0)
```

## 5. 课堂展示脚本

1. 展示手动启动需要三个终端：
   - `go run ./cmd/server/storage`
   - `go run ./cmd/server/game`
   - `go run ./cmd/server/gateway`
2. 展示 `exp5/game-cluster.yaml`，指出 3 个 Deployment 负责“我要几个实例”，3 个 Service 负责“别人如何找到我”。
3. 执行：

```bash
kubectl apply -f exp5/game-cluster.yaml
kubectl get pods -w
```

4. 用客户端移动玩家，展示链路：

```text
client -> gateway-service -> Gateway-Pod -> game-service -> Game-Pod -> storage-service -> Storage-Pod
```

5. 删除 game Pod，观察 Deployment 自动拉起新 Pod：

```bash
kubectl delete pod -l app=game
kubectl get pods -w
```

重启后继续请求，说明网关仍然访问 `game-service`，不需要知道新 Pod IP。

## 6. 清理

```bash
kubectl delete -f exp5/game-cluster.yaml
```
