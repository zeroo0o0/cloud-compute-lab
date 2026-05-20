# game-app 代码

本实验只保留“无状态网关”所需的最小应用：

- `gateway`：长连接入口；自己不保存会话，所有 session 都写入 Redis。
- `client`：每 1 秒发送一次 heartbeat；连接断开后携带同一个 token 自动重连。
- `internal/redismini`：用标准库实现的极小 Redis RESP 客户端，避免为了课堂实验再引入第三方依赖。

```text
game-app/
├── go.mod
├── internal/
│   └── redismini/
│       └── client.go
└── cmd/
    ├── client/
    │   └── main.go
    └── server/
        └── gateway/
            └── main.go
```
