# 实验七：弹性伸缩应对潮汐流量

本实验演示 `HorizontalPodAutoscaler`（HPA）如何根据 CPU 压力自动扩缩容：

- `game`：初始只有 1 个副本，`/burn` 接口会真实消耗 CPU。
- `loadgen`：按阶段逐步提高并发度，模拟“公会战”流量从上涨到爆发。
- `HPA`：当平均 CPU 使用率超过 50% 时，把 `game` 自动扩到更多副本；压力消失后，再自动缩回去。

课堂上要让学生看到两件事：

- 流量升高时，`game` Pod 数量会从 1 个逐步增加。
- 停止压测后，Pod 数量不会立刻下降，但等待一段时间后会自动缩回。

## 目录结构

```text
exp7/
├── README.md                    # 本实验说明文档
├── game-autoscale.yaml          # Deployment + Service + HPA
├── loadgen.yaml                 # 集群内压测器
├── build-images.sh              # 构建、标记并推送镜像
├── Dockerfile                   # 直接从 Go 源码构建镜像
├── Dockerfile.prebuilt          # 使用预编译二进制构建极简镜像
├── dist/                        # build-images.sh 生成的 Linux 二进制
└── game-app/                    # game 与 loadgen 源码
```

## 0. 登录云上 Kubernetes 集群

先 SSH 登录可以操作集群的服务器，例如节点 A：

```bash
ssh <用户名>@<节点地址>
```

进入本实验目录：

```bash
cd /path/to/ch6/exp7
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

HPA 依赖 metrics-server 提供 CPU 指标。继续确认指标服务可用：

```bash
kubectl top nodes
kubectl top pods
```

如果 `kubectl top` 暂时没有数据，HPA 的 `TARGETS` 可能显示 `<unknown>/50%`，需要先修好 metrics-server 或等待指标采集完成。

## 1. 构建并推送镜像

本实验统一使用节点 D 上的本地镜像仓库：

```text
10.0.2.12:5000
```

最终 Kubernetes YAML 使用的镜像是：

```text
10.0.2.12:5000/exp7/exp7-game:v1
10.0.2.12:5000/exp7/exp7-loadgen:v1
```

在 `exp7` 目录执行：

```bash
bash build-images.sh
```

脚本会直接使用当前 `exp7` 目录作为 Docker 构建上下文，并依次完成：

```bash
docker build -f Dockerfile.prebuilt --build-arg SERVICE=game -t exp7-game:v1 .
docker build -f Dockerfile.prebuilt --build-arg SERVICE=loadgen -t exp7-loadgen:v1 .

docker tag exp7-game:v1 10.0.2.12:5000/exp7/exp7-game:v1
docker tag exp7-loadgen:v1 10.0.2.12:5000/exp7/exp7-loadgen:v1

docker push 10.0.2.12:5000/exp7/exp7-game:v1
docker push 10.0.2.12:5000/exp7/exp7-loadgen:v1
```

确认本地 Docker 镜像：

```bash
docker images | grep exp7
```

## 2. 部署初始只有 1 个副本的 game

应用 YAML：

```bash
kubectl apply -f game-autoscale.yaml
```

查看 Deployment、HPA 和 Pod：

```bash
kubectl get deploy game
kubectl get hpa game
kubectl get pods -o wide
```

预期一开始只有 1 个 `game` Pod：

```text
NAME   READY   UP-TO-DATE   AVAILABLE
game   1/1     1            1
```

关键配置在 `game-autoscale.yaml`：

```yaml
resources:
  requests:
    cpu: 100m

averageUtilization: 50

behavior:
  scaleUp:
    stabilizationWindowSeconds: 0
    policies:
      - type: Pods
        value: 8
        periodSeconds: 15
  scaleDown:
    stabilizationWindowSeconds: 15
    policies:
      - type: Percent
        value: 100
        periodSeconds: 15
```

这表示：HPA 以 `requests.cpu` 为基准，当平均 CPU 利用率超过 50% 时，就会考虑扩容。为了让课堂上更快看到变化，本实验使用“快速演示模式”：扩容稳定窗口为 `0` 秒，单轮最多允许补到 `8` 个 Pod；缩容稳定窗口缩短为 `15` 秒，并允许每轮最多缩掉当前副本的 `100%`。

## 3. 开启分屏观察

建议开三个终端：

终端 A：看 Pod 数量变化

```bash
watch kubectl get pods -o wide
```

终端 B：看 HPA 判断

```bash
watch kubectl get hpa game
```

终端 C：看 CPU 指标

```bash
watch kubectl top pods
```

如果 `kubectl top pods` 没有数据，先不要启动压测，等 metrics-server 正常返回指标后再继续。

## 4. 制造“流量海啸”

启动集群内压测器：

```bash
kubectl apply -f loadgen.yaml
kubectl get pods -l app=loadgen
```

`loadgen` 会持续访问：

```text
http://game-service:8081/burn?ms=250
```

它不会一上来就把压力打满，而是按下面的节奏逐步加压：

```text
0s      2 workers
30s     4 workers
60s     8 workers
90s     16 workers
```

`/burn` 不是“假忙”，而是真的在容器里持续做 CPU 计算，所以可以推动 HPA 看到 CPU 压力。

几分钟内，终端里会逐步看到类似现象：

```text
game-xxxxx   1/1   Running
game-yyyyy   1/1   Running
game-zzzzz   1/1   Running
...
```

同时 `kubectl get hpa game` 会从：

```text
TARGETS   MINPODS   MAXPODS   REPLICAS
0%/50%    1         8         1
```

逐渐变成类似：

```text
TARGETS    MINPODS   MAXPODS   REPLICAS
180%/50%   1         8         4
```

## 5. 停止压测，观察自动缩容

停止压测器：

```bash
kubectl delete -f loadgen.yaml
```

继续观察：

```bash
watch kubectl get hpa game
watch kubectl get pods
```

注意：缩容仍然不会是毫秒级。HPA 仍按控制循环定期判断，只是本实验把缩容窗口压到 `15` 秒，因此压测停止后，课堂上通常会更快看到 `game` Pod 数量回落。

## 6. 课堂展示脚本

1. 展示 `game-autoscale.yaml`：
   - `replicas: 1`
   - `requests.cpu: 100m`
   - `averageUtilization: 50`
   - `minReplicas: 1`
   - `maxReplicas: 8`
   - `scaleUp.stabilizationWindowSeconds: 0`
   - `scaleUp.policies: 15 秒内最多增加 8 个 Pod`
   - `scaleDown.stabilizationWindowSeconds: 15`
   - `scaleDown.policies: 15 秒内最多缩掉当前副本的 100%`

2. 执行：

```bash
kubectl apply -f game-autoscale.yaml
kubectl get deploy game
kubectl get hpa game
```

3. 开分屏：

```bash
watch kubectl get pods
watch kubectl get hpa game
watch kubectl top pods
```

4. 启动压测：

```bash
kubectl apply -f loadgen.yaml
```

5. 让学生观察：

```text
-> CPU 逐步上升
-> HPA 判断超过阈值
-> Pod 数量逐步扩张
```

6. 停止压测：

```bash
kubectl delete -f loadgen.yaml
```

7. 等待并观察自动缩容。

## 7. 清理

```bash
kubectl delete -f loadgen.yaml --ignore-not-found
kubectl delete -f game-autoscale.yaml
```

确认资源已经删除：

```bash
kubectl get pods
kubectl get hpa
kubectl get svc
```
