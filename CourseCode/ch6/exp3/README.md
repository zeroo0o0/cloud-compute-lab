# 实验三：进入容器排查问题

## 目标

- 进入 `game-service` 容器
- 查看进程、端口、网络、日志
- 验证容器内能够访问 `storage`

## 前置条件

先进入 `exp1` 目录构建三层镜像：

```bash
cd /mnt/e/work/cloud-compute-book-code/CourseCode/ch6/exp1
docker build -f docker/storage.Dockerfile -t game-storage:v1.0 .
docker build -f docker/game.Dockerfile -t game-service:v1.0 .
```

以下步骤以 Linux / WSL 为主，不依赖实验四。

```bash
cd /mnt/e/work/cloud-compute-book-code/CourseCode/ch6/exp1
docker rm -f game-service storage >/dev/null 2>&1 || true
docker network rm game-network >/dev/null 2>&1 || true
docker volume rm game-data >/dev/null 2>&1 || true
docker network create game-network
docker run -d --name storage --network game-network \
  -e STORAGE_ADDR=0.0.0.0:8082 \
  -e STORAGE_LOG_PATH=/app/data/players.log \
  -v game-data:/app/data \
  game-storage:v1.0
docker run -d --name game-service --network game-network \
  -e GAME_ADDR=0.0.0.0:8081 \
  -e STORAGE_URL=http://storage:8082 \
  -v game-data:/app/data \
  -p 8081:8081 \
  game-service:v1.0
```

发送一次请求生成日志：

```bash
curl -X POST "http://127.0.0.1:8081/move" \
  -H "Content-Type: application/json" \
  -d '{"playerId":"demo-user","dx":1,"dy":0}'
```

## 命令

交互进入：

```bash
docker exec -it game-service /bin/sh
```

容器内执行：

```sh
ps aux
netstat -tlnp
ifconfig
ping -c 1 storage
cat /app/data/players.log
exit
```

单次执行：

```bash
docker exec game-service ps aux
docker exec game-service netstat -tlnp
docker exec game-service ifconfig
docker exec game-service ping -c 1 storage
docker exec game-service cat /app/data/players.log
```

## Windows PowerShell 补充

如在 Windows PowerShell 中演示，可使用与上面相同的容器命名和端口，仅将目录切换与 JSON 请求改写为 PowerShell 语法。

## 预期

- 可见 `game` 进程
- 可见 `8081` 监听
- `storage` 能解析并连通
- `players.log` 可读取
