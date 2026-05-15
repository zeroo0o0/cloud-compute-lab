# 飞升入定演示实验 —— 代码与演示说明

> 本目录对应第六章前四个实验。
> 实验一、二使用单容器镜像 `game-map0:v1.0`；实验三、四使用三层服务 `storage`、`game-service`、`gateway`。

---

## 目录结构

```text
ch6/
├── README.md
├── 飞升入定演示实验.md
├── exp1/
│   ├── go.mod
│   ├── README.md
│   ├── cmd/
│   │   ├── env_demo/
│   │   ├── client/
│   │   └── server/
│   └── docker/
│       ├── env_demo.Dockerfile
│       ├── config.yaml
│       ├── storage.Dockerfile
│       ├── game.Dockerfile
│       └── gateway.Dockerfile
├── exp2/
│   └── README.md
├── exp3/
│   └── README.md
└── exp4/
    ├── docker-compose.yml
    └── README.md
```

---

## 环境要求

| 依赖 | 说明 |
|------|------|
| Docker Desktop | 已安装并启动 |
| Docker Compose | 使用 `docker compose` 子命令 |
| Go | 用于运行 `exp1/cmd/env_demo` |

执行前建议确认：

```powershell
docker version
docker compose version
go version
```

---

## 实验关系

| 实验 | 主题 | 入口目录 | 主要产物 |
|------|------|----------|----------|
| 实验一 | 容器化与环境隔离 | `exp1` | `game-map0:v1.0`、三层服务镜像 |
| 实验二 | 数据卷持久化 | `exp2` | 宿主机日志目录 `data/game-logs` |
| 实验三 | 进入容器排查问题 | `exp3` | `docker exec` 排障命令 |
| 实验四 | Docker Compose 单机编排 | `exp4` | `docker-compose.yml`、网络、卷、三层服务 |

---

## 推荐顺序

1. 在 `exp1` 完成宿主机失败与容器成功，并构建全部镜像。
2. 在 `exp2` 使用宿主机目录挂载，验证日志持久化。
3. 在 `exp4` 启动三层服务。
4. 在 `exp3` 进入 `game-service` 容器执行排查命令。

---

## 运行约定

- `game-map0:v1.0` 使用宿主机端口 `8081`，容器端口 `8080`。
- `gateway` 默认使用宿主机端口 `18080`，容器端口 `8080`。
- 实验二开始前，如存在旧的 `game-0` 容器，应先移除。
- 实验三前，应先由实验四启动三层服务，并至少发送一次 `MOVE` 请求，确保生成 `players.log`。
- 实验四默认直接使用实验一已构建的镜像；如需重新构建，可手动执行 `docker compose build`。
