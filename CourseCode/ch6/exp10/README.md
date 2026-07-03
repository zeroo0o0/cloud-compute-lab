# 实验十：Serverless 事件驱动函数 (每日奖励结算)

## 对应教材

第 79-83 页

## 实验目标

验证"按需实例化"与"用完即走"的特性，直观感受代码即服务（FaaS）、冷启动与热启动差异以及零闲置成本的优势。

本实验仅保留 **阿里云函数计算 FC** 的部署与演示方式。

## 目录结构

```
exp10/
├── cmd/
│   └── client/
│       └── main.go          # 测试客户端：签到请求 + 并发压测（支持直连函数 URL）
├── functions/
│   └── daily_reward/
│       └── handler.py       # 签到奖励函数 (Python)
├── go.mod
└── README.md
```

## 前置条件

- 已开通阿里云函数计算 FC（Serverless 方式）

## 运行方式

### 阿里云函数计算 FC

> 此方式演示真正的 Serverless：Scale-to-Zero、冷启动、按调用计费。

#### 1) 开通函数计算

1. 打开 https://fcnext.console.aliyun.com/
2. 开通服务（按量付费，有免费额度）

#### 2) 安装 Serverless Devs 工具

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

#### 3) 创建函数项目

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

#### 4) 部署

```bash
cd exp10

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

#### 5) 验证

```bash
# 使用输出中的 URL
FUNC_URL="https://xxx.cn-shanghai.fc.aliyuncs.com/2023-03-03/functions/daily-reward/triggers/http-trigger"

# 方式 A：直接用 curl 测试

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

#### 6) 使用客户端（推荐）

客户端支持直接调用函数 URL（适合已部署的 FC 服务）：

```powershell
cd e:\workspace\goproject\cloud-compute-book-code\CourseCode\ch6\exp10\cmd\client
go run . -function-url "https://xxx.cn-shanghai.fc.aliyuncs.com/2023-03-03/functions/daily-reward/triggers/http-trigger"
```

如果已经编译过客户端，也可以直接运行：

```powershell
cd e:\workspace\goproject\cloud-compute-book-code\CourseCode\ch6\exp10\cmd\client
./client.exe -function-url "https://xxx.cn-shanghai.fc.aliyuncs.com/2023-03-03/functions/daily-reward/triggers/http-trigger"
```

#### 7) 在控制台观察 Serverless 特性

1. 打开 FC 控制台 → 函数计算 → 函数列表 → daily-reward
2. 观察以下指标：

| 特性 | 观察位置 | 预期 |
|------|---------|------|
| Scale-to-Zero | 函数详情 → 实例信息 | 无请求时实例数为 0 |
| 冷启动 | 调用日志 → 首次调用耗时 | 200-500ms |
| 热启动 | 调用日志 → 后续调用耗时 | 10-50ms |
| 自动扩容 | 监控面板 → 并发实例数 | 压测时自动增加 |

#### 8) 费用

函数计算有免费额度，课程实验通常不会产生费用：

| 项目 | 免费额度 | 超出单价 |
|------|---------|---------|
| 调用次数 | 100 万次/月 | ¥0.0133/万次 |
| 执行时间 | 40 万 GB·秒/月 | ¥0.00011108/GB·秒 |
| 外网出流量 | 无 | ¥0.50/GB |

#### 9) 清理

```bash
# 删除函数
s remove

# 或在控制台手动删除
```

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

## 关键概念

- **FaaS (Function as a Service)**：代码即服务，开发者只需关注函数逻辑
- **冷启动 (Cold Start)**：首次调用时需要初始化运行环境，有额外延迟
- **热启动 (Warm Start)**：运行环境已就绪，直接执行函数
- **Scale-to-Zero**：没有请求时不产生任何计算资源消耗
- **按需计费**：只为实际执行时间付费，空闲时段零成本

