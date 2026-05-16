# 实验七：弹性伸缩应对潮汐流量

本实验演示 `HorizontalPodAutoscaler`（HPA）如何根据 CPU 压力自动扩缩容：

- `game`：初始只有 1 个副本，`/burn` 接口会真实消耗 CPU。
- `loadgen`：按阶段逐步提高并发度，模拟“公会战”流量从上涨到爆发。
- `HPA`：当平均 CPU 使用率超过 50% 时，把 `game` 自动扩到更多副本；压力消失后，再自动缩回去。

课堂上要让学生看到两件事：

- 流量升高时，`game` Pod 数量会从 1 个逐步增加。
- 停止压测后，Pod 数量不会立刻下降，但等待一段时间后会自动缩回。

## 目录结构（当前）

```text
exp7/
├── README.md                    # 本实验说明文档
├── game-autoscale.yaml          # Deployment + Service + HPA
├── loadgen.yaml                 # 集群内压测器
├── build-images.sh              # WSL / Ubuntu 下构建镜像脚本
├── Dockerfile                   # 直接从 Go 源码构建镜像
├── Dockerfile.prebuilt          # 使用预编译二进制构建极简镜像
├── dist/                        # build-images.sh 生成的 Linux 二进制
└── game-app/                    # game 与 loadgen 源码
```

## 0. 准备 exp7 Minikube 集群

先打开 Ubuntu / WSL：

```powershell
wsl -d Ubuntu
```

进入项目目录：

```bash
cd "/mnt/你的项目目录/ch6"
```

新建并启动一个名为 `exp7` 的 Minikube 集群：

```bash
minikube start -p exp7
```

确认当前操作的是 `exp7` 集群：

```bash
kubectl config current-context
kubectl get nodes
```

## 1. 启用 metrics-server

HPA 需要先拿到 Pod 的资源指标。Minikube 可以直接启用 `metrics-server` 插件：

```bash
minikube -p exp7 addons enable metrics-server
```

等待一会儿后检查：

```bash
kubectl get pods -n kube-system
kubectl top pods
```

如果 `kubectl top pods` 能返回指标，说明 HPA 的“眼睛”已经睁开。

## 2. 构建镜像

### 方式 A：先在 WSL 编译，再用极简镜像打包（推荐）

```bash
bash exp7/build-images.sh
```

### 方式 B：直接使用 Dockerfile 从源码构建

```bash
docker build -f exp7/Dockerfile --build-arg TARGET=game -t exp7-game:v1 .
docker build -f exp7/Dockerfile --build-arg TARGET=loadgen -t exp7-loadgen:v1 .
```

两种方式任选其一。完成后，把镜像加载进 Minikube：

```bash
minikube -p exp7 image load exp7-game:v1
minikube -p exp7 image load exp7-loadgen:v1
```

## 3. 部署初始只有 1 个副本的 game

```bash
kubectl apply -f exp7/game-autoscale.yaml
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

## 4. 开启分屏观察

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

## 5. 制造“流量海啸”

启动集群内压测器：

```bash
kubectl apply -f exp7/loadgen.yaml
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

`/burn` 不是“假忙”，而是真的在容器里持续做 CPU 计算，所以可以推动 HPA 看到 CPU 压力。这样做的好处是，课堂上更容易观察到 CPU 逐步升高、HPA 再逐步扩容，而不是所有变化挤在一瞬间发生。

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

## 6. 停止压测，观察自动缩容

停止压测器：

```bash
kubectl delete -f exp7/loadgen.yaml
```

继续观察：

```bash
watch kubectl get hpa game
watch kubectl get pods
```

注意：缩容仍然不会是毫秒级。HPA 仍按控制循环定期判断，只是本实验把缩容窗口压到 `15` 秒，因此压测停止后，课堂上通常会更快看到 `game` Pod 数量回落。这个配置适合演示，不适合作为生产默认值。

## 7. 课堂展示脚本

1. 展示 `exp7/game-autoscale.yaml`：
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
kubectl apply -f exp7/game-autoscale.yaml
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
kubectl apply -f exp7/loadgen.yaml
```

5. 让学生观察：

```text
loadgen workers：2 -> 4 -> 8 -> 16
-> CPU 逐步上升
-> HPA 判断超过阈值
-> Deployment 增加副本
-> Pod 数量逐步扩张
```

6. 停止压测：

```bash
kubectl delete -f exp7/loadgen.yaml
```

7. 等待并观察自动缩容。

## 8. 清理

```bash
kubectl delete -f exp7/loadgen.yaml --ignore-not-found
kubectl delete -f exp7/game-autoscale.yaml
```

## 9. 常见问题

### HPA 一直显示 `<unknown>/50%`

通常说明资源指标还没准备好。先检查：

```bash
kubectl top pods
kubectl get pods -n kube-system
```

如果 `kubectl top pods` 还不能返回数据，再等一会儿后重试。

### Pod 一直不扩容

优先检查三件事：

```bash
kubectl get hpa game
kubectl top pods
kubectl logs deploy/loadgen
```

还要确认 `game-autoscale.yaml` 里已经配置了 `resources.requests.cpu`。没有 CPU request，HPA 就没有计算利用率的基准。

