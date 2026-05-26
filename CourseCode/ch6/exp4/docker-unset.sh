#!/bin/bash

# 1. 停止并强制删除所有相关容器 (忽略报错)
docker rm -f gateway game-service storage >/dev/null 2>&1 || true

# 2. 删除内部网络
docker network rm game-net >/dev/null 2>&1 || true

# 3. 删除共享数据卷 (注意：这会清空 /app/data 里的持久化数据)
docker volume rm game-data >/dev/null 2>&1 || true
