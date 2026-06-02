# 实验一：容器化与环境隔离

## 目标

- 宿主机运行失败
- 容器内运行成功
- 构建实验三、四所需的三层服务镜像

## 目录

```text
exp1/
├── cmd/env_demo/          # 单容器最小演示
├── cmd/client/            # 三层服务客户端
├── cmd/server/storage/    # storage
├── cmd/server/game/       # game-service
├── cmd/server/gateway/    # gateway
└── docker/
    ├── env_demo.Dockerfile
    ├── config.yaml
    ├── storage.Dockerfile
    ├── game.Dockerfile
    └── gateway.Dockerfile
```

## 步骤

进入目录：

```powershell
cd E:\work\cloud-compute-book-code\CourseCode\ch6\exp1
```

### 1. 宿主机运行失败

```powershell
go run ./cmd/env_demo
```

预期：

- 进程退出
- 提示缺少 `/etc/game/config.yaml`

### 2. 容器内运行成功

构建单容器镜像：

```powershell
docker build -f docker/env_demo.Dockerfile -t game-map0:v1.0 .
```

启动容器：

```powershell
docker run --rm game-map0:v1.0
```

预期：

- 输出 `Server Started`

停止方式：

- `Ctrl + C`

### 3. 构建三层服务镜像

```powershell
docker build -f docker/storage.Dockerfile -t game-storage:v1.0 .
docker build -f docker/game.Dockerfile -t game-service:v1.0 .
docker build -f docker/gateway.Dockerfile -t game-gateway:v1.0 .
```

## 检查

```powershell
docker images
```

应能看到：

- `game-map0:v1.0`
- `game-storage:v1.0`
- `game-service:v1.0`
- `game-gateway:v1.0`
