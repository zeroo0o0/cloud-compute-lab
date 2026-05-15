# 实验四：Docker Compose 单机多容器编排

## 目标

- 使用 `docker-compose.yml` 一键启动 `storage`、`game-service`、`gateway`
- 自动创建网络和数据卷

## 前置条件

先在 `exp1` 中构建三层镜像：

```powershell
docker build -f docker/storage.Dockerfile -t game-storage:v1.0 .
docker build -f docker/game.Dockerfile -t game-service:v1.0 .
docker build -f docker/gateway.Dockerfile -t game-gateway:v1.0 .
```

## 步骤

进入目录：

```powershell
cd E:\work\cloud-compute-book-code\CourseCode\ch6\exp4
```

如需清理手动启动的容器：

```powershell
docker rm -f gateway game-service storage 2>$null
```

启动服务：

```powershell
docker compose up -d
docker compose ps
```

如需重新构建镜像：

```powershell
docker compose build
docker compose up -d
```

查看日志：

```powershell
docker compose logs -f
```

停止服务：

```powershell
docker compose down
```

彻底清理数据：

```powershell
docker compose down -v
```

## 端口

- 默认宿主机端口：`18080`
- 如有冲突：

```powershell
$env:GATEWAY_HOST_PORT=28080
docker compose up -d
```

## 预期

- 三个服务均处于 `Up`
- `game-net` 与 `game-data` 自动创建
- `down` 不删除卷，`down -v` 删除卷
