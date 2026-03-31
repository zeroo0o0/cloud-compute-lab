# ch4/cmd 目录索引（不改程序版）

本目录仅做结构整理说明，不改动任何程序代码与运行路径。

## 目录总览

```text
cmd/
├─ exp1/
│  ├─ local_serial_loop_demo/
│  ├─ network_serial_server_demo/
│  ├─ network_goroutine_server_demo/
│  └─ network_event_driven_sync_demo/
├─ exp2/
│  ├─ wrong/
│  └─ right/
├─ exp3/
│  ├─ busy_wait/
│  └─ cond_wait/
├─ exp4/
│  ├─ channel_semaphore/
│  ├─ channel_timeout_lock/
│  └─ rw_mutex/
├─ exp5/
│  └─ sync_pool_demo/
└─ exp6/
   └─ storage_arch/
```

## 运行入口

- exp1:
  - `go run ./cmd/exp1/local_serial_loop_demo`
  - `go run ./cmd/exp1/network_serial_server_demo server`
  - `go run ./cmd/exp1/network_serial_server_demo client`
  - `go run ./cmd/exp1/network_goroutine_server_demo server`
  - `go run ./cmd/exp1/network_goroutine_server_demo client`
  - `go run ./cmd/exp1/network_event_driven_sync_demo server`
  - `go run ./cmd/exp1/network_event_driven_sync_demo client`
- exp2:
  - `go run ./cmd/exp2/wrong`
  - `go run ./cmd/exp2/right`
- exp3:
  - `go run ./cmd/exp3/busy_wait`
  - `go run ./cmd/exp3/cond_wait`
- exp4:
  - `go run ./cmd/exp4/channel_semaphore`
  - `go run ./cmd/exp4/channel_timeout_lock`
  - `go run ./cmd/exp4/rw_mutex`
- exp5:
  - `go run ./cmd/exp5/sync_pool_demo`
- exp6:
  - `go run ./cmd/exp6/storage_arch`

## 命名说明

- `exp1` 现在使用四个入口，对应“本地串行主循环版”“网络串行收包版”“网络独立线程收包版”“网络事件驱动 + 增量同步版”。
- `exp2` 与 `exp3` 保持“错误版/修复版”的结构，便于课堂中前后对照。
- `exp4` 将 Channel 技巧拆成“信号量”和“超时锁”两个独立入口，再配合 `RWMutex` 演示锁粒度。
- `exp5` 聚焦 `sync.Pool` 的性能优化演示，不额外引入数据库连接池代码。
- `exp6` 使用真实 Redis 与 PostgreSQL 演示分层存储架构。
- 当前目录保持原路径不变，避免影响既有讲义、截图和运行命令。
