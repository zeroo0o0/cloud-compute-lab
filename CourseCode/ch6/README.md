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
├── exp_aliyun_docker/
│   ├── docker-compose.yml
│   └── README.md
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

建议以 `WSL2 / Ubuntu` 作为主环境，`Windows PowerShell` 作为补充。

Linux / WSL 建议先确认：

```bash
docker version
docker compose version
go version
```

Windows PowerShell 可补充确认：

```powershell
docker version
docker compose version
go version
```

如遇 Docker 代理问题，可先清理环境变量：

```bash
unset HTTP_PROXY HTTPS_PROXY NO_PROXY
```

如遇 `docker build` 证书或代理配置异常，可暂时移走 Docker CLI 配置：

```bash
mv ~/.docker/config.json ~/.docker/config.json.bak
```

---

## 实验关系

| 实验 | 主题 | 入口目录 | 主要产物 |
|------|------|----------|----------|
| 实验一 | 容器化与环境隔离 | `exp1` | `game-map0:v1.0`、三层服务镜像 |
| 实验二 | 数据卷持久化 | `exp2` | 宿主机日志目录 `data/game-logs` |
| 实验三 | 进入容器排查问题 | `exp3` | `docker exec` 排障命令 |
| 实验四 | Docker Compose 单机编排 | `exp4` | `docker-compose.yml`、网络、卷、三层服务 |
| 云端扩展 | 环境冲突与 Docker 修复 | `exp_aliyun_docker` | 云端部署失败 + 容器统一环境 |

---

## 推荐顺序

1. 在 `exp1` 完成宿主机失败与容器成功，并构建全部镜像。
2. 在 `exp2` 使用宿主机目录挂载，验证日志持久化。
3. 在 `exp3` 独立启动 `storage` 与 `game-service`，完成容器内排查。
4. 在 `exp4` 使用 Compose 完成单机多容器编排。
5. （可选）在 `exp_aliyun_docker` 复现云端环境冲突并用 Docker 修复。

---

## 运行约定

- `game-map0:v1.0` 使用宿主机端口 `8081`，容器端口 `8080`。
- `gateway` 默认使用宿主机端口 `18080`，容器端口 `8080`。
- 实验二开始前，如存在旧的 `game-0` 容器，应先移除。
- 实验三不依赖实验四；如存在旧的 `storage`、`game-service`、`gateway`、`game-network`、`game-data`，应先清理。
- 实验四默认直接使用实验一已构建的镜像；如需重新构建，可手动执行 `docker compose build`。
