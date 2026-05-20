# Lab4 云上部署说明

本目录用于将 Lab4 拆分版战场系统部署到 TKE / Kubernetes。

当前采用的方案是：

- `gateway` 独立 Deployment
- `coordinator` 独立 Deployment + PVC
- `green / cave / ruins` 三张地图各自独立 Deployment

---

## 一、部署内容

- 网关镜像：`/Users/chenjunjie/Downloads/Lab/Lab4/Dockerfile.cloud-gateway`
- 协调器镜像：`/Users/chenjunjie/Downloads/Lab/Lab4/Dockerfile.cloud-coordinator`
- 地图镜像：`/Users/chenjunjie/Downloads/Lab/Lab4/Dockerfile.cloud-map`
- Kubernetes 清单目录：`/Users/chenjunjie/Downloads/Lab/Lab4/deploy/split-k8s`
- 一键部署脚本：`/Users/chenjunjie/Downloads/Lab/Lab4/deploy/split-apply.sh`

部署后会生成 5 个核心工作负载：

1. `lab3-gateway`
2. `lab3-coordinator`
3. `lab3-map-green`
4. `lab3-map-cave`
5. `lab3-map-ruins`

说明：当前资源名称仍沿用 `lab3-*`，这是为了和现有镜像、端口与服务习惯保持兼容；目录与实验划分已经独立到 Lab4。

---

## 二、先决条件

需要满足：

1. 已连上 TKE / Kubernetes 集群
2. 集群支持 `LoadBalancer`
3. 集群支持 `PVC`
4. 已准备可推送镜像仓库（推荐腾讯云 TCR）
5. `kubectl get nodes` 正常

---

## 三、镜像地址

当前默认示例：

- `ccr.ccs.tencentyun.com/lab4/gateway:latest`
- `ccr.ccs.tencentyun.com/lab4/coordinator:latest`
- `ccr.ccs.tencentyun.com/lab4/map:latest`

---

## 四、构建并推送镜像

```bash
cd /Users/chenjunjie/Downloads/Lab/Lab4

docker build -f Dockerfile.cloud-gateway -t GATEWAY_IMAGE .
docker build -f Dockerfile.cloud-coordinator -t COORDINATOR_IMAGE .
docker build -f Dockerfile.cloud-map -t MAP_IMAGE .

docker push GATEWAY_IMAGE
docker push COORDINATOR_IMAGE
docker push MAP_IMAGE
```

---

## 五、部署到集群

推荐直接使用脚本：

```bash
cd /Users/chenjunjie/Downloads/Lab/Lab4
chmod +x ./deploy/split-apply.sh
./deploy/split-apply.sh \
  ccr.ccs.tencentyun.com/lab4/gateway:latest \
  ccr.ccs.tencentyun.com/lab4/coordinator:latest \
  ccr.ccs.tencentyun.com/lab4/map:latest \
  100042155098 \
  '你的 TCR 密码'
```

脚本会自动执行：

1. 创建命名空间（如果不存在）
2. 创建 `tcr-pull` 镜像拉取 secret
3. `kubectl apply -k deploy/split-k8s`
4. 更新镜像地址
5. 等待各个 Deployment 滚动完成
6. 打印 `pods / svc / pvc`

---

## 六、手工模式

如果不使用脚本，可以手工执行：

```bash
kubectl create namespace lab3-split
kubectl -n lab3-split create secret docker-registry tcr-pull \
  --docker-server=ccr.ccs.tencentyun.com \
  --docker-username=100042155098 \
  --docker-password='你的 TCR 密码'

kubectl apply -k /Users/chenjunjie/Downloads/Lab/Lab4/deploy/split-k8s
```

之后检查：

```bash
kubectl get pods -n lab3-split -o wide
kubectl get svc -n lab3-split
kubectl get pvc -n lab3-split
```

---

## 七、公网入口

客户端入口服务：

```bash
kubectl get svc lab3-gateway -n lab3-split
```

如果 `EXTERNAL-IP` 或腾讯云 CLB 域名已经分配成功，就可以用：

```text
公网地址:9310
```

连接客户端。

---

## 八、当前特点

优点：

1. 已经不是单进程内嵌三张图，而是三张地图分别部署
2. gateway / coordinator / map-* 已经是独立工作负载
3. coordinator 有独立 PVC，可持久化账号与会话数据
4. 2PC 战利品转移已经走真实跨服务调用

当前限制：

1. 存储还没有继续外置到 Redis / MySQL
2. coordinator 仍然是单副本控制面
3. 资源名称还保留 `lab3-*` 兼容前序部署
