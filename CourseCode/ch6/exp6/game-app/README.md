# game-app 代码

本实验只保留一个 `game` HTTP 服务，用来把注意力集中在 Kubernetes `Service` 本身：

- `GET /move`：返回一条成功响应，供集群内外访问演示。
- `GET /healthz`：供 Kubernetes 就绪探针使用。

```text
game-app/
├── go.mod
└── cmd/
    └── server/
        └── game/
            └── main.go
```
