# game-app 代码

本实验展示「网关（Gateway）— 无状态编排器（Game）— 状态存储（Storage）」三进程解耦的演示思路。

## 目录结构（当前）

```text
game-app/
├── go.mod                       # 独立模块：exp5/game-app
├── README.md                    # ← 本文件
├── internal/                    # 本实验内部复用代码
│   ├── proto/                   # 客户端、网关、game、storage 共用消息类型
│   │   └── types.go
│   └── render/                  # 客户端地图渲染工具
│       └── render.go
└── cmd/                         # 程序入口目录
    ├── client/                  # 演示客户端：TCP 连接 Gateway
    │   └── main.go              # 交互式命令行客户端，发送 GET/MOVE 并渲染地图
    └── server/                  # 服务端入口目录
        ├── gateway/             # 网关进程：TCP 文本协议 -> HTTP 转换
        │   └── main.go          # 解析 GET/MOVE，转发到 game，并回写结果
        ├── game/                # 游戏逻辑服务：无状态编排
        │   └── main.go          # 读旧状态 -> 应用逻辑 -> 写回 storage
        └── storage/             # 状态存储服务：内存实现、非持久化
            └── main.go          # 提供 /get 和 /set，作为状态唯一权威源
```

## 一、启动服务（示例）

需要 4 个终端（3 个服务 + 1 个客户端）。在本目录下运行：

终端 A：Storage（8082）
```powershell
cd exp5/game-app
go run ./cmd/server/storage
```

终端 B：Game（8081）
```powershell
cd exp5/game-app
go run ./cmd/server/game
```

终端 C：Gateway（8080）
```powershell
cd exp5/game-app
go run ./cmd/server/gateway
```

终端 D：Client
```powershell
cd exp5/game-app
go run ./cmd/client
```

## 二、协议与交互（简要）

- 客户端到网关（TCP 文本协议）
  - `GET <clientId>` — 查询玩家当前位置（网关 -> game -> storage -> 返回位置）
  - `MOVE <clientId> <direction>` — 请求移动，`<direction>` 支持：`w/a/s/d`（`w` 表示向上、`s` 向下）

- 客户端行为
  1. 每轮自动发 `GET` 获取当前位置并显示 `当前位置: x=?,y=?`。
  2. 提示输入方向或 `q` 退出；输入合法则发送 `MOVE` 请求并等待响应。

示例（客户端可见）输出：
```
请输入方向 (w/a/s/d) 或 q 退出: s
移动成功
当前位置: x=0,y=1
+----------------------+
|. . . . . . . . . . . |
|P . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
|. . . . . . . . . . . |
+----------------------+
```

## 三、故障演示场景（简要）

- `game` 挂掉——停止 `game`，保持 `gateway` 与 `storage` 运行；客户端发 `MOVE`，网关会返回 `RESULT err layer=game reason=unreachable`。
- 重启 `game` 恢复——`storage` 保存了状态时，`game` 重启后可继续从 `storage` 恢复玩家位置。


## 四、重要说明（Storage 非持久化）

本实现中的 `storage` 使用进程内内存保存玩家坐标，未实现磁盘或外部数据库持久化。结果：

- 当 `storage` 进程重启时，已保存的玩家坐标将丢失；从玩家视角来看，位置会被重置（回到默认初始值）。
- 如果希望保留玩家位置，请替换 `storage` 为持久化实现（如文件、SQLite、Redis 等）。


## 五、环境变量（可选）

- `GATEWAY_ADDR`（默认 `127.0.0.1:8080`）
- `GAME_URL`（默认 `http://127.0.0.1:8081`）
- `GAME_ADDR`（默认 `127.0.0.1:8081`）
- `STORAGE_URL`（默认 `http://127.0.0.1:8082`）
- `STORAGE_ADDR`（默认 `127.0.0.1:8082`）
- `CLIENT_SERVER_URL`（默认 `127.0.0.1:8080`）
- `CLIENT_ID`（默认 `client-<pid>`）

---

