# Lab 4 · 云上拆分部署与 TKE 编排

> **定位**：把本地版 Lab3 的多地图战场系统拆成真正的多服务，并部署到云上 Kubernetes / TKE 集群。

Lab4 不再区分 `complete/` 和 `student/`，当前目录直接提供一套云上部署代码与清单。

---

## 一、实验目标

Lab4 重点考查：

1. **服务拆分**
   - 将单机模拟集群拆成真实服务：`gateway / coordinator / map-*`
2. **云上部署**
   - 构建镜像
   - 推送到 TCR
   - 使用 TKE / Kubernetes 部署
3. **三张地图独立工作负载**
   - `green`
   - `cave`
   - `ruins`
4. **存储与持久化**
   - 当前由 `coordinator + PVC` 负责冷数据和会话落盘
5. **跨服务协同**
   - 网关接入
   - 协调器路由
   - 地图服务承载主状态
   - 2PC 负责跨节点战利品转移

---

## 二、目录结构

```text
Lab4/
├── README.md
├── go.mod
├── cloud/
├── cloudapi/
├── cmd/
│   ├── admin/
│   ├── client/
│   ├── cloud-gateway/
│   ├── cloud-coordinator/
│   └── cloud-map/
├── protocol/
├── storage/
├── twopc/
├── world/
├── Dockerfile.cloud-gateway
├── Dockerfile.cloud-coordinator
├── Dockerfile.cloud-map
└── deploy/
    ├── README.md
    ├── split-apply.sh
    └── split-k8s/
```

---

## 三、当前服务拆分

### 1. gateway
负责：
- 客户端入口
- 登录 / 注册
- 操作转发
- 状态回传

### 2. coordinator
负责：
- 路由决策
- 地图 owner / replica 管理
- 世界 Boss 全局状态
- 故障切换
- 数据持久化
- 2PC 协调

### 3. map-green / map-cave / map-ruins
分别负责：
- 每张地图自己的玩家状态
- NPC 推进
- 宝物刷新
- 战斗与掉落
- checkpoint / restore / promote

---

## 四、部署环境

推荐环境：

- 腾讯云 TKE
- 腾讯云 TCR
- 至少 3 个工作节点
- 支持 `LoadBalancer`
- 支持 `PVC`

---

## 五、镜像构建

三个镜像：

- `ccr.ccs.tencentyun.com/lab4/gateway:latest`
- `ccr.ccs.tencentyun.com/lab4/coordinator:latest`
- `ccr.ccs.tencentyun.com/lab4/map:latest`

构建示例：

```bash
cd /Users/chenjunjie/Downloads/Lab/Lab4
docker build -f Dockerfile.cloud-gateway -t ccr.ccs.tencentyun.com/lab4/gateway:latest .
docker build -f Dockerfile.cloud-coordinator -t ccr.ccs.tencentyun.com/lab4/coordinator:latest .
docker build -f Dockerfile.cloud-map -t ccr.ccs.tencentyun.com/lab4/map:latest .
```

---

## 六、部署方式

查看详细步骤：

- [deploy/README.md](/Users/chenjunjie/Downloads/Lab/Lab4/deploy/README.md)

一键部署脚本：

```bash
cd /Users/chenjunjie/Downloads/Lab/Lab4
./deploy/split-apply.sh \
  ccr.ccs.tencentyun.com/lab4/gateway:latest \
  ccr.ccs.tencentyun.com/lab4/coordinator:latest \
  ccr.ccs.tencentyun.com/lab4/map:latest \
  100042155098 \
  '你的TCR密码'
```

---

## 七、当前实验边界

当前 Lab4 已经完成：

1. 网关独立 Deployment
2. 协调器独立 Deployment
3. 三张地图独立 Deployment
4. `LoadBalancer` 暴露网关
5. `PVC` 挂给协调器保存数据
6. 镜像可直接推送到 TCR 并在 TKE 拉起

当前尚未继续外置：

1. Redis
2. MySQL
3. coordinator 高可用副本

也就是说，这版 Lab4 已经能完成“上云拆分部署”，但还不是最终完整云原生高可用版。
