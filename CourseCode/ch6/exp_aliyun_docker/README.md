# 阿里云 Docker 环境冲突演示（基于 exp1 游戏服务）

## 目标

- 直接运行真实的三层服务（storage / game / gateway）和客户端
- 本地开发机运行正常
- 云服务器部署 server 时因环境冲突失败
- 使用 Docker 统一环境后恢复运行
- 串联 exp1-4 的核心流程：启动 -> 持久化 -> 进入容器排查 -> compose 编排

## 目录

```text
exp_aliyun_docker/
├── cmd/client/            # 三层服务客户端
├── cmd/server/storage/    # storage
├── cmd/server/game/       # game-service（包含 Python 版本检查）
├── cmd/server/gateway/    # gateway
├── docker/
│   ├── storage.Dockerfile
│   ├── game.Dockerfile
│   └── gateway.Dockerfile
└── docker-compose.yml
```

## 环境冲突说明（云端失败点）

`game-service` 启动时会检查 Python 3.11+（`tomllib`），云端只有 Python 3.10：

```text
python env check failed: ...
ModuleNotFoundError: No module named 'tomllib'
```

这代表真实的版本冲突：代码在本机通过，但云端环境较旧导致服务启动失败。

```powershell
cd .\CourseCode\ch6\exp_aliyun_docker
```

## 1. 本机运行（预期成功）

在三个终端中依次启动：

```powershell
go run ./cmd/server/storage
```

```powershell
go run ./cmd/server/game
```

```powershell
go run ./cmd/server/gateway
```

再启动客户端：

```powershell
go run ./cmd/client
```

## 2. 云端部署 server（预期失败）

在云服务器上执行：

```bash
go run ./cmd/server/game
```

预期：Python 3.10 缺少 `tomllib`，服务启动失败。

## 3. Docker 方式修复（串联 exp1 + exp2）

### 3.1 构建镜像

```powershell
docker build -f docker/storage.Dockerfile -t game-storage:v1.0 .
docker build -f docker/game.Dockerfile -t game-service:v1.0 .
docker build -f docker/gateway.Dockerfile -t game-gateway:v1.0 .
```

### 3.2 启动并挂载数据卷（持久化）

```powershell
mkdir data
docker network create game-net
docker run -d --name storage --network game-net -v ${PWD}/data:/app/data game-storage:v1.0
docker run -d --name game-service --network game-net -e STORAGE_URL=http://storage:8082 game-service:v1.0
docker run -d --name gateway --network game-net -p 18080:8080 -e GAME_URL=http://game-service:8081 game-gateway:v1.0
```

客户端连接到网关：

```powershell
$env:CLIENT_SERVER_URL="127.0.0.1:18080"
go run ./cmd/client
```

> `data/players.log` 会在宿主机持久化，符合 exp2 要求。

## 4. 进入容器排查（exp3）

```powershell
docker exec -it game-service sh
```

示例排查：

- `ps aux`
- `netstat -tlnp`
- `cat /app/data/players.log`

## 5. Docker Compose 一键编排（exp4）

```powershell
docker compose up -d
docker compose ps
docker compose logs -f
```

关闭：

```powershell
docker compose down
```

如需删除卷：

```powershell
docker compose down -v
```
