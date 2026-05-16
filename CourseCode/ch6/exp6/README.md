# 实验六：K8s Service 内外部访问

本实验演示同一个 `game` 服务如何同时被两类入口访问：

- `game-service`：`ClusterIP`，供集群内部访问。
- `game-service-external`：`NodePort`，供集群外部访问。

课堂上要让学生看到两件事：

- 在集群内部，可以直接通过稳定的 Service 名称 `http://game-service:8081` 访问服务。
- 在集群外部，NodePort 会把服务暴露为 `NodeIP:30080`；在多节点集群中，访问任意一个 Node 的 `30080`，都能转发到同一组后端 Pod。

## 目录结构（当前）

```text
exp6/
├── README.md                    # 本实验说明文档
├── game-service.yaml            # Deployment + ClusterIP Service
├── game-svc-external.yaml       # NodePort Service
├── build-images.sh              # WSL / Ubuntu 下构建镜像脚本
├── Dockerfile                   # 直接从 Go 源码构建镜像
├── Dockerfile.prebuilt          # 使用预编译二进制构建极简镜像
├── dist/                        # build-images.sh 生成的 Linux 二进制
└── game-app/                    # 待部署的 game 服务源码
```

## 0. 准备 exp6 Minikube 集群

先打开 Ubuntu / WSL：

```powershell
wsl -d Ubuntu
```

进入本项目目录。比如Windows 的 `C:\ch6` 在 WSL 里通常对应：

```bash
cd "/mnt/c/ch6"
```

新建并启动一个名为 `exp6` 的双节点 Minikube 集群：

```bash
minikube start -p exp6 --nodes 2
```

确认当前操作的是 `exp6` 集群：

```bash
kubectl config current-context
kubectl get nodes -o wide
```

看到当前上下文是 `exp6`，并且节点状态类似下面这样，就说明可以继续：

```text
exp6

NAME       STATUS   ROLES           AGE   VERSION   INTERNAL-IP
exp6       Ready    control-plane   ...   ...       192.168.x.x
exp6-m02   Ready    <none>          ...   ...       192.168.x.x
```

## 1. 构建镜像

### 方式 A：先在 WSL 编译，再用极简镜像打包（推荐）

在 Ubuntu / WSL 里执行 Bash 脚本。它会先把 Go 服务编译成 Linux 静态二进制，再使用 `Dockerfile.prebuilt` 打镜像。

```bash
bash exp6/build-images.sh
```

### 方式 B：直接使用 Dockerfile 从源码构建

如果你不想在 WSL 本地安装 Go，也可以让 Docker 在构建镜像时完成编译：

```bash
docker build -f exp6/Dockerfile -t exp6-game:v1 .
```

两种方式任选其一即可。完成后，把镜像加载进 Minikube：

```bash
minikube -p exp6 image load exp6-game:v1
```

## 2. 创建集群内部入口：ClusterIP

先应用 `game-service.yaml`：

```bash
kubectl apply -f exp6/game-service.yaml
```

查看 Pod 和 Service：

```bash
kubectl get pods -o wide
kubectl get svc game-service
```

预期能看到：

```text
NAME                    READY   STATUS    ...
game-xxxxxxxxxx-xxxxx   1/1     Running   ...

NAME           TYPE        CLUSTER-IP      PORT(S)
game-service   ClusterIP   10.x.x.x        8081/TCP
```

这里的关键点是：`game-service` 的类型是 `ClusterIP`，它只提供集群内部访问入口。

## 3. 在集群内部访问 game-service

启动一个临时 debug 容器：

```bash
kubectl run -it --rm debug --image=busybox --restart=Never -- /bin/sh
```

进入 debug 容器后，执行：

```sh
wget -qO- http://game-service:8081/move
```

成功时会看到：

```json
{"ok":true,"service":"game","message":"move accepted"}
```

退出 debug 容器：

```sh
exit
```

这一步说明：在集群内部，不需要知道 Pod IP，只要知道 Service 名称就能访问。

## 4. 创建集群外部入口：NodePort

再应用 `game-svc-external.yaml`：

```bash
kubectl apply -f exp6/game-svc-external.yaml
kubectl get svc game-service game-service-external
```

预期能看到：

```text
NAME                    TYPE        CLUSTER-IP      PORT(S)
game-service            ClusterIP   10.x.x.x        8081/TCP
game-service-external   NodePort    10.x.x.x        8081:30080/TCP
```

这里的关键点是：

- `game-service` 仍然是集群内部入口。
- `game-service-external` 是外部入口，把同一组 `game` Pod 暴露到每个 Node 的 `30080` 端口。

## 5. 在集群外部访问任意 Node 的 NodePort

先查看所有 Node 的 IP：

```bash
kubectl get nodes -o wide
```

假设两个节点分别是：

```text
exp6       192.168.49.2
exp6-m02   192.168.49.3
```

那么标准访问方式就是分别访问两个 Node 的同一个 `30080` 端口：

```bash
curl http://192.168.49.2:30080/move
curl http://192.168.49.3:30080/move
```

两个地址都成功时，会看到相同的返回：

```json
{"ok":true,"service":"game","message":"move accepted"}
```

这说明：即使请求从不同 Node 进入，`NodePort` 也能把它们转发到同一个 Service 后端。

### 如果直接访问 NodeIP:30080 失败

如果你使用的是 `docker` driver，尤其是在 macOS / Windows / WSL 等环境下，Minikube 节点可能位于 Docker 的内部网络中，当前终端未必能直接访问这个节点 IP。此时并不是 NodePort 创建失败，而是：

```text
NodePort 已经存在，
但当前终端到 Minikube 节点 IP 的网络路径不通。
```

这时改用 Minikube 提供的辅助访问方式：

```bash
minikube service game-service-external -p exp6
```

执行后，Minikube 会输出一个当前环境可访问的地址，并尝试在浏览器中打开它。这个地址的端口可能不是 `30080`，因为它是 Minikube 临时建立的本地转发入口；真正的 Kubernetes `NodePort` 仍然是 YAML 中配置的 `30080`。

输出示例：

```text
|-----------|-----------------------|-------------|--------------------------|
| NAMESPACE |         NAME          | TARGET PORT |           URL            |
|-----------|-----------------------|-------------|--------------------------|
| default   | game-service-external | http/8081   | http://192.168.85.2:30080 |
|-----------|-----------------------|-------------|--------------------------|
🏃  Starting tunnel for service game-service-external.
|-----------|-----------------------|-------------|------------------------|
| NAMESPACE |         NAME          | TARGET PORT |          URL           |
|-----------|-----------------------|-------------|------------------------|
| default   | game-service-external |             | http://127.0.0.1:38805 |
|-----------|-----------------------|-------------|------------------------|
```

这里第一张表里的 `192.168.85.2:30080` 是原始 NodePort 地址；后面的 `127.0.0.1:38805` 是 Minikube 为当前环境临时建立的可访问通道。

可以把这层关系理解成：

```text
你的终端 -> Minikube 临时访问地址 -> NodePort 30080 -> game Pod 8081
```

然后对 Minikube 输出的地址追加 `/move` 进行访问，例如：

```bash
curl http://127.0.0.1:xxxxx/move
```

## 6. 对比两个 Service 背后的后端

查看 Service：

```bash
kubectl get svc game-service game-service-external
```

再查看它们各自转发到哪些后端：

```bash
kubectl get endpoints game-service game-service-external
```

预期会看到两个 Service 后面都指向同一个 `game` Pod IP，例如：

```text
NAME                    ENDPOINTS
game-service            10.244.0.5:8081
game-service-external   10.244.0.5:8081
```

这就是本实验最重要的观察点：

```text
集群内访问：game-service:8081
集群外访问：Node-A:30080 / Node-B:30080
                          \ /
                       同一组 game Pod
```

## 7. 课堂展示脚本

1. 展示 `exp6/game-service.yaml`：
   - `Deployment` 负责真正运行 `game` Pod。
   - `game-service` 的 `type: ClusterIP` 表示它只服务于集群内部访问。
2. 执行：

```bash
kubectl apply -f exp6/game-service.yaml
kubectl get pods -o wide
kubectl get svc game-service
```

3. 启动 debug 容器并访问：

```bash
kubectl run -it --rm debug --image=busybox --restart=Never -- /bin/sh
wget -qO- http://game-service:8081/move
```

4. 展示 `exp6/game-svc-external.yaml`：
   - `type: NodePort`
   - `nodePort: 30080`
5. 执行：

```bash
kubectl apply -f exp6/game-svc-external.yaml
kubectl get svc game-service game-service-external
kubectl get nodes -o wide
curl http://<Node-A-IP>:30080/move
curl http://<Node-B-IP>:30080/move
```

6. 如果直接访问失败，再执行：

```bash
minikube service game-service-external -p exp6
```

并对输出地址追加 `/move` 访问，说明外部客户端仍然能到达同一后端。
7. 最后执行：

```bash
kubectl get endpoints game-service game-service-external
```

说明两个访问入口不同，但背后转发到的是同一个 `game` Pod。

## 8. 清理

```bash
kubectl delete -f exp6/game-svc-external.yaml
kubectl delete -f exp6/game-service.yaml
```

## 9. 常见问题

### debug 容器里访问失败

先确认 `game` Pod 已经进入 `Running`：

```bash
kubectl get pods
kubectl describe pod -l app=game
```

再确认 `game-service` 已经创建：

```bash
kubectl get svc game-service
kubectl get endpoints game-service
```

### 外部 `curl` 访问失败

先确认 NodePort Service 已经创建：

```bash
kubectl get svc game-service-external
```

如果你直接访问 `NodeIP:30080` 不通，不代表 Service 配置失败。若你使用的是 `docker` driver，尤其是在 macOS / Windows / WSL 等环境下，节点 IP 可能位于 Docker 内部网络，当前终端不能直接路由到它。

这时改用：

```bash
minikube service game-service-external -p exp6
```

由 Minikube 建立可访问通道后，再访问它输出的地址。
