# 实验二代码（分片原型）

本实验演示在微服务基础上引入横向分片（Sharding），通过路由机制将不同玩家/地图的流量分发到对应实例。

## 目录结构（当前）

```text
exp2/
├── go.mod                       # 独立模块：ch5/exp2
├── README.md                    # ← 本文件
├── internal/                    # 本实验内部复用代码
│   ├── proto/                   # 客户端、网关、game、storage 共用消息类型
│   │   └── types.go
│   └── render/                  # 客户端地图渲染工具
│       └── render.go
└── cmd/
    ├── client/                  # 演示客户端：TCP 连接 Gateway
    │   └── main.go              # 交互式命令行客户端，发送 GET/MOVE 并渲染地图
    └── server/
        ├── gateway/             # 网关：TCP 文本协议 -> HTTP 转换
        │   └── main.go          # 解析 GET/MOVE，按 MapID 路由到 Game
        ├── game/                # 游戏计算分片：无状态编排
        │   └── main.go          # 按 UserID 路由到 Storage，读旧状态 -> 应用逻辑 -> 写回
        └── storage/             # 存储分片：内存实现、非持久化
            └── main.go          # 提供 /get 和 /set，作为状态唯一权威源
```

## 一、启动服务（示例）

需要 6 个终端（4 个服务 + 1 个网关 + 1 个客户端）。在本目录下运行：

终端 A：Storage-0（8082）
```powershell
cd exp2
go run ./cmd/server/storage
```

终端 B：Storage-1（8084）
```powershell
cd exp2
go run ./cmd/server/storage
```

终端 C：Game-0（8081）
```powershell
cd exp2
go run ./cmd/server/game
```

终端 D：Game-1（8083）
```powershell
cd exp2
go run ./cmd/server/game
```

终端 E：Gateway（8080）
```powershell
cd exp2
go run ./cmd/server/gateway
```

终端 F：Client
```powershell
cd exp2
go run ./cmd/client
```

> 说明：Storage/Game 支持“同命令启动两次自动占用不同默认分片端口”，课堂演示不需要额外参数。

## 二、协议与交互（简要）

- 客户端到网关（TCP 文本协议）
  - `GET <userID> <mapID>` — 查询玩家当前位置
  - `MOVE <userID> <mapID> <direction>` — 请求移动，`<direction>` 支持：`w/a/s/d`

- 路由手段（核心）
   - `MapID` 按 `% 2` 路由到 `Game-0/Game-1`
   - `UserID` 按 `% 2` 路由到 `Storage-0/Storage-1`

- 客户端行为
  1. 每轮可触发一次 GET 获取当前位置并显示地图。
  2. 输入方向或 `map <id>`、`user <id>`，客户端会自动发起对应请求。

示例（客户端可见）输出：
```
[gateway] map_id=0 -> Game-0
[game] user_id=1 -> Storage-1
RESULT ok position=(x=0,y=1)
```

## 三、成功判定（和实验目标对应）

1. 在客户端设置 `map 0`，用户设为 `user 1`（或默认用户直接继续）。
2. 观察输出应包含：
   - `[gateway] map_id=0 -> Game-0`（因为 `0 % 2 = 0`）
   - `[game] user_id=1 -> Storage-1`（因为 `1 % 2 = 1`）

再执行：

1. 输入 `map 1`
2. 观察输出应切到：
   - `[gateway] map_id=1 -> Game-1`
3. 输入 `user 2`
4. 观察输出应切到：
   - `[game] user_id=2 -> Storage-0`


## 四、重要说明（Storage 非持久化）

本实现中的 `storage` 使用进程内内存保存玩家坐标，未实现磁盘或外部数据库持久化。结果：

- 当 `storage` 进程重启时，已保存的玩家坐标将丢失；从玩家视角来看，位置会被重置（回到默认初始值）。
- 如果希望保留玩家位置，请替换 `storage` 为持久化实现（如文件、SQLite、Redis 等）。

## 五、环境变量（可选）

- `CLIENT_SERVER_URL`（默认 `127.0.0.1:8080`）
- `CLIENT_USER_ID`（默认 `1`）
- `CLIENT_MAP_ID`（默认 `0`）
