# exp10 阿里云部署指南：Serverless 事件驱动函数

> 本实验提供两种部署方式：
> - **方式 A**：部署到 K8s 集群（与 exp9/11/Lab4 共享集群）
> - **方式 B**：使用阿里云函数计算 FC（真正的 Serverless，推荐用于演示 Scale-to-Zero）

---

## 方式 A：部署到 K8s 集群

> 前置条件：已完成 [集群搭建](../cluster-setup/README.md)。

### A.1 资源需求

| 项目 | 值 |
|------|-----|
| Namespace | `exp10` |
| Pod 数 | 1 |
| CPU | 100m request / 500m limit |
| 内存 | 128Mi request / 256Mi limit |
| 特殊依赖 | Python3（镜像内置） |
| 外部访问 | NodePort 30090 |

### A.2 构建并推送镜像

```bash
cd CourseCode/ch6/exp10

# 构建镜像（包含 Go runtime + Python3 + 函数代码）
docker build -t ch6-exp10-runtime:latest .

# 推送 ACR
docker tag ch6-exp10-runtime:latest crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp10-runtime:latest
docker push crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp10-runtime:latest
```

### A.3 修改 K8s 配置

#### k8s/faas-runtime-deployment.yaml

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: faas-runtime
  namespace: exp10
  labels:
    app: faas-runtime
spec:
  replicas: 1
  selector:
    matchLabels:
      app: faas-runtime
  template:
    metadata:
      labels:
        app: faas-runtime
    spec:
      containers:
      - name: runtime
        image: crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute/ch6-exp10-runtime:latest
        imagePullPolicy: Always
        ports:
        - containerPort: 9000
        env:
        - name: PORT
          value: "9000"
        resources:
          requests:
            cpu: "100m"
            memory: "128Mi"
          limits:
            cpu: "500m"
            memory: "256Mi"
```

#### k8s/faas-runtime-service.yaml

```yaml
apiVersion: v1
kind: Service
metadata:
  name: faas-runtime
  namespace: exp10
spec:
  type: NodePort
  selector:
    app: faas-runtime
  ports:
  - port: 9000
    targetPort: 9000
    nodePort: 30090
```

### A.4 部署

```bash
kubectl create namespace exp10
kubectl apply -f k8s/
kubectl -n exp10 wait --for=condition=ready pod -l app=faas-runtime --timeout=60s
kubectl -n exp10 get pods
```

### A.5 验证

```bash
export NODE_IP=<任意节点公网IP>

# 查看 runtime 状态
curl http://$NODE_IP:30090/stats

# 加载函数
curl -X POST http://$NODE_IP:30090/load \
  -H "Content-Type: application/json" \
  -d '{"name":"daily_reward","runtime":"python","handler":"handler.py","entry":"handler","memory_mb":128,"timeout_s":30}'

# 调用函数（冷启动）
curl -X POST "http://$NODE_IP:30090/invoke?function=daily_reward" \
  -H "Content-Type: application/json" \
  -d '{"event":{"player_id":"player_001","action":"signin"}}'

# 再次调用（热启动，响应更快）
curl -X POST "http://$NODE_IP:30090/invoke?function=daily_reward" \
  -H "Content-Type: application/json" \
  -d '{"event":{"player_id":"player_002","action":"signin"}}'

# 使用客户端压测
cd CourseCode/ch6/exp10
go run cmd/client/main.go -runtime http://$NODE_IP:30090
```

### A.6 清理

```bash
kubectl delete namespace exp10
```

---

## 方式 B：使用阿里云函数计算 FC（推荐）

> 此方式演示真正的 Serverless：Scale-to-Zero、冷启动、按调用计费。

### B.1 开通函数计算

1. 打开 https://fcnext.console.aliyun.com/
2. 开通服务（按量付费，有免费额度）

### B.2 安装 Serverless Devs 工具

```bash
# 安装 Node.js（如未安装）
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt-get install -y nodejs

# 安装 Serverless Devs
npm install -g @serverless-devs/s

# 配置阿里云认证
s config add --provider alibabacloud
# 按提示输入：
#   AccessKey ID: <从 RAM 控制台获取>
#   AccessKey Secret: <从 RAM 控制台获取>
#   Region: cn-shanghai
```

### B.3 创建函数项目

创建目录结构：

```
exp10-fc/
├── s.yaml
└── src/
    └── handler.py
```

**s.yaml**：

```yaml
edition: 3.0.0
name: daily-reward-app
access: default

vars:
  region: cn-shanghai

resources:
  daily-reward:
    component: fc3
    props:
      region: ${vars.region}
      functionName: daily-reward
      description: "每日签到奖励函数 - 课程实验"
      runtime: python3.10
      handler: handler.handler
      cpu: 0.35
      memorySize: 256
      diskSize: 512
      timeout: 30
      environmentVariables:
        PYTHON_ENV: production
      code:
        - src/
      triggers:
        - triggerName: http-trigger
          triggerType: http
          triggerConfig:
            authType: anonymous
            methods:
              - GET
              - POST
```

**src/handler.py**：

```python
#!/usr/bin/env python3
"""每日签到奖励函数 - 阿里云函数计算版本"""
import json
import hashlib
from datetime import datetime

def handler(environ, start_response):
    """
    阿里云 FC HTTP 触发器入口
    符合 WSGI 规范
    """
    # 解析请求
    method = environ.get('REQUEST_METHOD', 'GET')
    path = environ.get('PATH_INFO', '/')

    # 从 query string 或 body 获取参数
    query_string = environ.get('QUERY_STRING', '')
    try:
        request_body_size = int(environ.get('CONTENT_LENGTH', 0))
    except (ValueError):
        request_body_size = 0
    request_body = environ['wsgi.input'].read(request_body_size) if request_body_size > 0 else b''

    # 解析事件数据
    event = {}
    if request_body:
        try:
            event = json.loads(request_body)
        except json.JSONDecodeError:
            pass

    player_id = event.get("player_id", "unknown")
    action = event.get("action", "signin")
    timestamp = event.get("timestamp", datetime.now().isoformat())

    # 生成奖励
    seed = hashlib.md5(f"{player_id}_{timestamp[:10]}".encode()).hexdigest()
    gold = int(seed[:4], 16) % 100 + 10
    exp = int(seed[4:8], 16) % 500 + 50
    streak_day = int(seed[8:10], 16) % 7 + 1
    streak_bonus = streak_day * 10 if streak_day > 3 else 0

    result = {
        "player_id": player_id,
        "action": action,
        "timestamp": timestamp,
        "reward": {
            "gold": gold + streak_bonus,
            "exp": exp,
            "streak_day": streak_day,
            "streak_bonus": streak_bonus,
        },
        "message": f"签到成功！获得 {gold + streak_bonus} 金币，{exp} 经验",
        "runtime": "aliyun-fc"
    }

    # 返回 HTTP 响应
    status = '200 OK'
    response_body = json.dumps(result, ensure_ascii=False).encode('utf-8')
    response_headers = [
        ('Content-Type', 'application/json; charset=utf-8'),
        ('Content-Length', str(len(response_body)))
    ]
    start_response(status, response_headers)
    return [response_body]
```

### B.4 部署

```bash
cd exp10-fc

# 部署到阿里云
s deploy

# 输出示例：
# daily-reward:
#   functionName: daily-reward
#   functionArn: acs:fc:cn-shanghai:xxx:functions/daily-reward
#   triggers:
#     - triggerName: http-trigger
#       url: https://xxx.cn-shanghai.fc.aliyuncs.com/2023-03-03/functions/daily-reward/triggers/http-trigger
```

### B.5 验证

```bash
# 使用输出中的 URL
FUNC_URL="https://xxx.cn-shanghai.fc.aliyuncs.com/2023-03-03/functions/daily-reward/triggers/http-trigger"

# 首次调用（冷启动，较慢）
curl -X POST $FUNC_URL \
  -H "Content-Type: application/json" \
  -d '{"player_id":"player_001","action":"signin","timestamp":"2024-01-15T10:00:00Z"}'

# 再次调用（热启动，很快）
curl -X POST $FUNC_URL \
  -H "Content-Type: application/json" \
  -d '{"player_id":"player_002","action":"signin","timestamp":"2024-01-15T10:00:00Z"}'

# 并发压测
for i in $(seq 1 50); do
  curl -s -X POST $FUNC_URL \
    -H "Content-Type: application/json" \
    -d "{\"player_id\":\"player_$i\",\"action\":\"signin\"}" &
done
wait
```

### B.6 在控制台观察 Serverless 特性

1. 打开 FC 控制台 → 函数计算 → 函数列表 → daily-reward
2. 观察以下指标：

| 特性 | 观察位置 | 预期 |
|------|---------|------|
| Scale-to-Zero | 函数详情 → 实例信息 | 无请求时实例数为 0 |
| 冷启动 | 调用日志 → 首次调用耗时 | 200-500ms |
| 热启动 | 调用日志 → 后续调用耗时 | 10-50ms |
| 自动扩容 | 监控面板 → 并发实例数 | 压测时自动增加 |

### B.7 费用

函数计算有免费额度，课程实验通常不会产生费用：

| 项目 | 免费额度 | 超出单价 |
|------|---------|---------|
| 调用次数 | 100 万次/月 | ¥0.0133/万次 |
| 执行时间 | 40 万 GB·秒/月 | ¥0.00011108/GB·秒 |
| 外网出流量 | 无 | ¥0.50/GB |

### B.8 清理

```bash
# 删除函数
s remove

# 或在控制台手动删除
```
