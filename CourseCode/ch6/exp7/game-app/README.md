# game-app 代码

本实验保留两个入口：

- `game`：提供 `/burn`，每次请求都真实消耗一小段 CPU。
- `loadgen`：持续并发访问 `game-service`，制造“潮汐流量”。

```text
game-app/
├── go.mod
└── cmd/
    ├── loadgen/
    │   └── main.go
    └── server/
        └── game/
            └── main.go
```
