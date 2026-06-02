# 实验六：K8s Service 内外部访问

本实验演示同一个 `game` 服务如何同时被两类入口访问：

- `game-service`：`ClusterIP`，供集群内部访问。
- `game-service-external`：`NodePort`，供集群外部访问。

课堂上要让学生看到两件事：

- 在集群内部，可以直接通过稳定的 Service 名称 `http://game-service:8081` 访问服务。
- 在集群外部，NodePort 会把服务暴露为 `NodeIP:30086`；在多节点集群中，访问任意一个 Node 的 `30086`，都能转发到同一组后端 Pod。

## 目录结构

```text
exp6/
├── README.md                    # 本实验说明文档
├── game-service.yaml            # Deployment + ClusterIP Service
├── game-svc-external.yaml       # NodePort Service
├── build-images.sh              # 构建、标记并推送镜像
├── Dockerfile                   # 直接从 Go 源码构建镜像
├── Dockerfile.prebuilt          # 使用预编译二进制构建极简镜像
├── dist/                        # build-images.sh 生成的 Linux 二进制
└── game-app/                    # 待部署的 game 服务源码
```

## 0. 登录云上 Kubernetes 集群

先 SSH 登录可以操作集群的服务器，例如节点 A：

```bash
ssh <用户名>@k8s-a
```

进入本项目目录：

```bash
cd /path/to/ch6
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

如果集群已经安装 metrics-server，可以顺手确认指标服务可用：

```bash
kubectl top nodes
kubectl top pods
```

本实验不依赖 HPA，`kubectl top` 失败不会影响 exp6 的核心流程。

## 1. 构建并推送镜像

本实验统一使用节点 D 上的本地镜像仓库：

```text
10.0.2.12:5000
```

最终 Kubernetes YAML 使用的镜像是：

```text
10.0.2.12:5000/exp6/exp6-game:v1
```

使用脚本构建、tag、push

在项目根目录执行：

```bash
bash exp6/build-images.sh
```

脚本会依次完成：

```bash
docker build -f exp6/Dockerfile.prebuilt -t exp6-game:v1 .
docker tag exp6-game:v1 10.0.2.12:5000/exp6/exp6-game:v1
docker push 10.0.2.12:5000/exp6/exp6-game:v1
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
game-service-external   NodePort    10.x.x.x        8081:30086/TCP
```

这里的关键点是：

- `game-service` 仍然是集群内部入口。
- `game-service-external` 是外部入口，把同一组 `game` Pod 暴露到每个 Node 的 `30086` 端口。

如果提示 `provided port is already allocated`，说明集群里已有其他 Service 占用了 `30086`。先清理对应实验，或者临时把 `exp6/game-svc-external.yaml` 中的 `nodePort` 改成未占用端口。

## 5. 在集群外部访问任意 Node 的 NodePort

先查看所有 Node 的 IP：

```bash
kubectl get nodes -o wide
```

假设几个节点的 IP 是：

```text
k8s-a   10.0.2.9
k8s-b   10.0.2.10
k8s-c   10.0.2.11
k8s-d   10.0.2.12
```

那么标准访问方式就是分别访问不同 Node 的同一个 `30086` 端口：

```bash
curl http://10.0.1.10:30086/move
curl http://10.0.2.10:30086/move
curl http://10.0.2.11:30086/move
curl http://10.0.2.12:30086/move
```

如果从公网访问，请使用云服务器公网 IP，并确认安全组已经放行 TCP `30086`。

两个或多个地址都成功时，会看到相同的返回：

```json
{"ok":true,"service":"game","message":"move accepted"}
```

这说明：即使请求从不同 Node 进入，`NodePort` 也能把它们转发到同一个 Service 后端。

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
集群外访问：Node-A:30086 / Node-B:30086 / Node-C:30086
                          \       |       /
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
   - `nodePort: 30086`

5. 执行：

```bash
kubectl apply -f exp6/game-svc-external.yaml
kubectl get svc game-service game-service-external
kubectl get nodes -o wide
curl http://<Node-A-IP>:30086/move
curl http://<Node-B-IP>:30086/move
```

6. 最后执行：

```bash
kubectl get endpoints game-service game-service-external
```

说明两个访问入口不同，但背后转发到的是同一个 `game` Pod。

## 8. 清理

```bash
kubectl delete -f exp6/game-svc-external.yaml
kubectl delete -f exp6/game-service.yaml
```

确认资源已经删除：

```bash
kubectl get pods
kubectl get svc
```
