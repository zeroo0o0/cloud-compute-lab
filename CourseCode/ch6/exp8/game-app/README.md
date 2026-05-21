# game-app 代码

本实验在“无状态网关 + Redis session”之外，补回真实游戏链路：

- `gateway`：长连接入口；自己不保存会话，所有 session 都写入 Redis。
- `game`：无状态游戏逻辑层，从 storage 读坐标、计算移动、再写回 storage。
- `storage`：玩家位置的唯一状态源。
- `client`：可交互游戏测试客户端；后台自动发送 heartbeat，用来演示重连恢复。
- `internal/redismini`：用标准库实现的极小 Redis RESP 客户端，避免为了课堂实验再引入第三方依赖。
- `internal/proto`、`internal/render`：复用实验五的游戏消息结构和地图渲染。

```text
game-app/
├── go.mod
├── internal/
│   ├── proto/
│   ├── redismini/
│   └── render/
└── cmd/
    ├── client/
    │   └── main.go
    └── server/
        ├── gateway/
        ├── game/
        └── storage/
```
