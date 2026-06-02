#!/bin/bash

# 1. 准备基础设施 (网络和数据卷)
docker network create game-net
docker volume create game-data

# 2. 手动按顺序构建三个镜像
docker build -t game-storage:v1.0 -f ../exp1/docker/storage.Dockerfile ../exp1
docker build -t game-service:v1.0 -f ../exp1/docker/game.Dockerfile ../exp1
docker build -t game-gateway:v1.0 -f ../exp1/docker/gateway.Dockerfile ../exp1

# 3. 启动 Storage 节点 (最底层依赖)
docker run -d \
  --name storage \
  --network game-net \
  -e STORAGE_ADDR=0.0.0.0:8082 \
  -e STORAGE_LOG_PATH=/app/data/players.log \
  -v game-data:/app/data \
  game-storage:v1.0

# 4. 启动 Game 逻辑节点 (依赖 Storage)
docker run -d \
  --name game-service \
  --network game-net \
  -e GAME_ADDR=0.0.0.0:8081 \
  -e STORAGE_URL=http://storage:8082 \
  -v game-data:/app/data \
  game-service:v1.0

# 5. 启动 Gateway 网关节点 (依赖 Game，并暴露端口)
# 注意：Bash 原生支持 ${VAR:-18080} 这种环境变量默认值语法
docker run -d \
  --name gateway \
  --network game-net \
  -e GATEWAY_ADDR=0.0.0.0:8080 \
  -e GAME_URL=http://game-service:8081 \
  -p ${GATEWAY_HOST_PORT:-18080}:8080 \
  game-gateway:v1.0
