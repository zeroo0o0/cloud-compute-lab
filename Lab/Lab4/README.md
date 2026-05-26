# Lab4：自建 Kubernetes 游戏部署实验

> 使用范围说明：本项目仅用于课程教学、实验训练和自有/授权 Kubernetes 集群内的功能验证。文档中的地址、端口、命令和并发参数均为实验示例，不包含真实凭据、口令、访问令牌或生产环境地址。请勿将本项目中的测试脚本、部署方式或访问配置用于任何未授权的第三方系统或公网服务。
>
> 安全提醒：提交仓库前请确认没有包含 kubeconfig、SSH 私钥、云厂商 AK/SK、Redis 密码、Token、真实用户数据或其他敏感信息。

本实验要求你把一个分布式游戏部署到自己的 Kubernetes 集群上，并让它具备 HPA 自动扩缩容、基础故障恢复和游戏状态一致性。

本次实验不提供 Dockerfile 和 Kubernetes YAML。你需要自己完成容器化、镜像分发、Kubernetes 资源编写和调优。评分脚本只检查最终集群运行结果，不检查你的 Dockerfile/YAML 是否采用某种固定写法。

## 1. 你要完成什么

最终效果：

```text
命名空间：lab4
访问入口：<自有或授权集群的 Node 访问地址>:30910
业务组件：gateway / coordinator / map-green / map-cave / map-ruins
状态组件：redis
HPA：5 个业务 Deployment 均开启，min=1，max=10
状态：Redis 保存实验用户状态、临时会话、地图 checkpoint、leader 租约
```

你需要自己补齐：

```text
Dockerfile
Kubernetes Deployment / StatefulSet / Service / HPA / RBAC 等 YAML
镜像构建、导入或推送流程
必要的资源 requests/limits、readinessProbe、livenessProbe、preStop
```

你不需要修改测试脚本；可以修改业务代码、Dockerfile、YAML 和部署方式来争取更高分。

## 2. 目录说明

```text
cmd/cloud-gateway        TCP 游戏入口
cmd/cloud-coordinator   游戏协调器，处理登录、移动、切图、交易等逻辑
cmd/cloud-map           地图服务，同一个程序通过环境变量区分 green/cave/ruins
cmd/client              交互式客户端
cmd/admin               管理/状态查看工具
cmd/gateway-loadtest    负载测试客户端，scoretest 内部也会用类似逻辑
cloud/                  coordinator/map 的核心逻辑
cluster/                Kubernetes 生命周期、leader、pod deletion-cost 等支持代码
redisx/                 Redis 客户端封装
storage/                用户、session、checkpoint 存储
test/                   基础测试和竞技评分脚本
```

## 3. 必须满足的资源名

评分脚本会按下面的名字查找资源，请保持一致：

```text
Deployment: lab4-gateway
Deployment: lab4-coordinator
Deployment: lab4-map-green
Deployment: lab4-map-cave
Deployment: lab4-map-ruins
StatefulSet: lab4-redis

Service: lab4-gateway
Service: lab4-coordinator
Service: lab4-map-green
Service: lab4-map-cave
Service: lab4-map-ruins
Service: lab4-redis

HPA: lab4-gateway
HPA: lab4-coordinator
HPA: lab4-map-green
HPA: lab4-map-cave
HPA: lab4-map-ruins
```

`lab4-gateway` 需要暴露 NodePort：

```text
containerPort: 9310
nodePort: 30910
```

## 4. 程序入口和端口

建议构建 3 类业务镜像：

```text
gateway：./cmd/cloud-gateway
coordinator：./cmd/cloud-coordinator
map：./cmd/cloud-map
```

服务端口约定：

```text
gateway TCP 游戏端口：9310
gateway HTTP 健康检查端口：9311
coordinator HTTP 端口：9320
map HTTP 端口：9400
redis 端口：6379
```

你可以用多阶段 Dockerfile 编译静态二进制，也可以用 Go 镜像直接运行；评分只看授权实验集群内的最终服务是否可用、是否可扩缩、是否能恢复。

## 5. 关键环境变量

gateway 至少需要：

```text
LAB4_GATEWAY_ADDR=0.0.0.0:9310
LAB4_GATEWAY_HEALTH_ADDR=0.0.0.0:9311
LAB4_COORDINATOR_URL=http://lab4-coordinator:9320
LAB4_NAMESPACE=lab4
```

coordinator 至少需要：

```text
LAB4_COORDINATOR_ADDR=:9320
LAB4_STATE_BACKEND=redis
LAB4_REDIS_ADDR=lab4-redis:6379
LAB4_REDIS_PREFIX=lab4
LAB4_GREEN_URL=http://lab4-map-green:9400
LAB4_CAVE_URL=http://lab4-map-cave:9400
LAB4_RUINS_URL=http://lab4-map-ruins:9400
LAB4_GREEN_NODE_ID=map-green
LAB4_CAVE_NODE_ID=map-cave
LAB4_RUINS_NODE_ID=map-ruins
LAB4_NAMESPACE=lab4
```

map 服务至少需要：

```text
LAB4_MAP_LISTEN_ADDR=:9400
LAB4_STATE_BACKEND=redis
LAB4_REDIS_ADDR=lab4-redis:6379
LAB4_REDIS_PREFIX=lab4
LAB4_MAP_ID=green/cave/ruins
LAB4_NODE_ID=map-green/map-cave/map-ruins
LAB4_COMPONENT=map-green/map-cave/map-ruins
LAB4_NAMESPACE=lab4
```

如果要支持 Pod 回收保护，业务 Pod 需要在授权实验集群内访问 Kubernetes API。通常需要按最小权限原则配置 ServiceAccount、Role 和 RoleBinding，使业务 Pod 仅能更新自身所需的 annotations。

## 6. 建议部署流程

1. 准备一个可用 Kubernetes 集群，并确认所有节点 `Ready`。

```bash
kubectl get nodes -o wide
kubectl get pods -A
```

2. 确认 metrics-server 可用。HPA 评分依赖 `kubectl top`。

```bash
kubectl top nodes
kubectl top pods -A
```

3. 编写 Dockerfile，构建 gateway、coordinator、map 镜像。

4. 将镜像推送到镜像仓库，或导入到所有会运行 Pod 的节点。

5. 编写并应用 Kubernetes YAML。

6. 检查部署结果。

```bash
kubectl -n lab4 get pods,hpa -o wide
kubectl -n lab4 get svc
kubectl -n lab4 get statefulset
```

7. 用客户端或 admin 工具验证游戏入口。请仅连接你自己或课程授权的实验集群。

```bash
go run ./cmd/admin 状态 <节点访问地址>:30910
go run ./cmd/client <节点访问地址>:30910
```

## 7. 基础测试

基础测试只验证是否部署成功、功能是否完整，不计入竞技分。

Linux/macOS：

```bash
./test/run-autotest.sh
```

Windows PowerShell：

```powershell
.\test\run-autotest.ps1
```

如果脚本提示输入：

```text
Gateway address, for example 203.0.113.10:30910:
```

请输入你自己的或课程授权的实验集群入口，例如：

```text
<节点访问地址>:30910
```

如果本机没有配置 kubeconfig，脚本会自动退回远程连接模式，提示输入授权实验集群的主节点地址、远程登录用户、端口和游戏入口地址。请不要把密码、私钥或其他凭据写入 README 或提交到仓库。

## 8. 评分与负载测试说明

以下脚本仅用于自有或课程授权 Kubernetes 集群中的 HPA 弹性验证和稳定性评估。运行前请确认 Gateway 地址属于你的实验环境，避免误连他人服务。


竞技评分满分 100 分：

```text
空闲不误扩：10
低压稳定性：15
高压弹性扩容：25
状态一致性：20
故障恢复：20
资源纪律：10
```

运行：

```bash
./test/run-scoretest.sh
```

脚本会持续输出阶段日志，不是静默等待。默认计时窗口约为：

```text
空闲观察：30s
低并发负载测试：45s
高压自校准：每轮 30s
高并发负载测试：120s
进度日志：约每 5s 输出一次
```

常用参数：

```bash
LAB4_GATEWAY_ADDR=<节点访问地址>:30910 ./test/run-scoretest.sh
LAB4_SCORE_FAST=1 ./test/run-scoretest.sh
LAB4_SKIP_CHAOS=1 ./test/run-scoretest.sh
LAB4_SCORE_HIGH_CLIENTS=300 ./test/run-scoretest.sh
```

调试时推荐先用：

```bash
LAB4_SCORE_FAST=1 LAB4_SKIP_CHAOS=1 ./test/run-scoretest.sh
```

评分脚本不按绝对 TPS 或网络延迟打分，避免把本机性能、集群硬件和网络质量算进成绩。高并发项会先在当前授权实验集群里自校准；如果无法形成明确 HPA 压力，会显示 `WAIVED`，该项不计入总分。

注意：竞技评分会在授权实验集群内真实触发 HPA 扩容。重复评分前请先等几分钟，让 HPA 缩回稳定状态，否则“空闲不误扩”会受到上一轮负载测试残留副本影响。

## 9. 观察 HPA

负载测试或评分时可以另开一个终端观察：

```bash
watch -n 0.5 'kubectl -n lab4 get pods,hpa -o wide'
```

HPA 生效时通常会看到：

```text
TARGETS 接近或超过 CPU 目标值
REPLICAS 从 1 增加到更多
新增 Pod 进入 Running
```

## 10. 常见问题

`scoretest` 看起来卡住了：

```text
正常情况下脚本会每隔几秒输出阶段日志。
如果长时间没有日志，请检查 kubectl 是否能访问集群、Gateway 地址是否可连通。
```

HPA 一直是 `<unknown>`：

```text
metrics-server 不可用，或者 Deployment 没有配置 CPU requests。
```

Pod 是 `ImagePullBackOff`：

```text
节点拉不到镜像。请检查镜像名、imagePullPolicy、镜像仓库权限，或确认镜像已导入到对应节点。
```

Redis 要不要 HPA：

```text
不建议。Redis 在本实验中是单实例状态组件，评分脚本也会检查 Redis 不应配置 HPA。
```

为什么需要 readinessProbe 和 preStop：

```text
缩容或删除 Pod 时，readinessProbe 用于停止接收新流量，preStop 用于进入 drain 并保存实验状态，减少玩家感知。
```
