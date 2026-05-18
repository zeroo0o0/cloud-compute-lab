# 实验十：Serverless 事件驱动函数 (每日奖励结算)

## 对应教材

第 79-83 页

## 实验目标

验证"按需实例化"与"用完即走"的特性，直观感受代码即服务（FaaS）、冷启动与热启动差异以及零闲置成本的优势。

> 本实验提供两种部署方式：
> - **方式 A**：部署到 K8s 集群（与 exp9/11/Lab4 共享集群）
> - **方式 B**：使用阿里云函数计算 FC（真正的 Serverless，推荐用于演示 Scale-to-Zero）
>
> 详细部署指南见 [deploy-aliyun.md](deploy-aliyun.md)。

## 目录结构

```
exp10/
├── cmd/
│   ├── runtime/
│   │   └── main.go          # FaaS 运行时：函数加载、调用、冷启动模拟
│   └── client/
│       └── main.go          # 测试客户端：签到请求 + 并发压测
├── functions/
│   └── daily_reward/
│       └── handler.py       # 签到奖励函数 (Python)
├── k8s/
│   ├── faas-runtime-deployment.yaml  # Runtime Deployment
│   └── faas-runtime-service.yaml     # NodePort Service (30090)
├── Dockerfile
├── deploy.sh                # 一键部署脚本
├── deploy-aliyun.md         # 阿里云部署指南
├── go.mod
└── README.md
```

## 前置条件

- 已完成 [集群搭建](../cluster-setup/README.md)（K8s 方式）
- 或已开通阿里云函数计算 FC（Serverless 方式）

## 运行方式

### 方式一：本地运行（快速验证）

```bash
cd CourseCode/ch6/exp10

# 终端 1：启动 FaaS Runtime
go run cmd/runtime/main.go

# 终端 2：运行测试客户端
go run cmd/client/main.go
```

### 方式二：K8s 部署

```bash
cd CourseCode/ch6/exp10

# 一键部署
bash deploy.sh
```

手动部署：

```bash
# 1. 构建并推送镜像
docker build --platform linux/amd64 -t ch6-exp10-runtime:latest .
docker tag ch6-exp10-runtime:latest crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp10-runtime:latest
docker push crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp10-runtime:latest

# 2. 部署
kubectl create namespace exp10
kubectl apply -f k8s/

# 3. 验证
export NODE_IP=<任意节点公网IP>
curl http://$NODE_IP:30090/stats

# 4. 手动测试
# 加载函数
curl -X POST http://$NODE_IP:30090/load \
  -H "Content-Type: application/json" \
  -d '{"name":"daily_reward","runtime":"python","handler":"handler.py","entry":"handler","memory_mb":128,"timeout_s":30}'

# 调用函数
curl -X POST "http://$NODE_IP:30090/invoke?function=daily_reward" \
  -H "Content-Type: application/json" \
  -d '{"event":{"player_id":"player_001","action":"signin"}}'

# 5. 使用客户端
go run cmd/client/main.go -runtime http://$NODE_IP:30090
```

### 方式三：阿里云函数计算 FC

详见 [deploy-aliyun.md](deploy-aliyun.md) 中的"方式 B"章节。

## 预期结果

### 冷启动 vs 热启动

| 调用次序 | 状态 | 响应时间 | 原因 |
|---------|------|---------|------|
| 第 1 次  | 冷启动 | 200-500ms | 需要启动 Python 解释器并加载脚本 |
| 第 2+ 次 | 热启动 | 10-50ms   | 解释器进程已预热 |

### 并发压测结果

```
Total: 100
OK: 100 | Fail: 0
Cold Starts: 1        (只有首次是冷启动)
Duration: ~3s
RPS: ~30
```

## API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/load` | 加载函数配置 |
| POST | `/invoke?function=name` | 调用函数 |
| GET  | `/stats` | 查看运行统计 |
| GET  | `/warmup?function=name` | 预热函数 |

## 关键概念

- **FaaS (Function as a Service)**：代码即服务，开发者只需关注函数逻辑
- **冷启动 (Cold Start)**：首次调用时需要初始化运行环境，有额外延迟
- **热启动 (Warm Start)**：运行环境已就绪，直接执行函数
- **Scale-to-Zero**：没有请求时不产生任何计算资源消耗
- **按需计费**：只为实际执行时间付费，空闲时段零成本

## 清理

```bash
kubectl delete namespace exp10
```
