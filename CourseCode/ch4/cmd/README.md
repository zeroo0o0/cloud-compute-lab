# ch4/cmd 目录索引（不改程序版）

本目录仅做结构整理说明，不改动任何程序代码与运行路径。

## 目录总览

```text
cmd/
├─ exp1/
│  ├─ single_thread_demo/
│  └─ goroutine_demo/
├─ exp2/
│  ├─ wrong/
│  └─ right/
├─ exp3/
│  ├─ busy_wait/
│  └─ cond_wait/
├─ exp4/
│  ├─ channel_tricks/
│  └─ rw_mutex/
├─ exp5/
│  └─ sync_pool_demo/
└─ exp6/
   └─ storage_arch/
```

## 运行入口

- exp1:
  - `go run ./cmd/exp1/single_thread_demo`
  - `go run ./cmd/exp1/goroutine_demo`
- exp2:
  - `go run ./cmd/exp2/wrong`
  - `go run ./cmd/exp2/right`
- exp3:
  - `go run ./cmd/exp3/busy_wait`
  - `go run ./cmd/exp3/cond_wait`
- exp4:
  - `go run ./cmd/exp4/channel_tricks`
  - `go run ./cmd/exp4/rw_mutex`
- exp5:
  - `go run ./cmd/exp5/sync_pool_demo`
- exp6:
  - `go run ./cmd/exp6/storage_arch`

## 命名说明

- `exp1` 使用两个入口对照展示“单线程阻塞版”和“Goroutine 改进版”。
- `exp2` 与 `exp3` 保持“错误版/修复版”的结构，便于课堂中前后对照。
- `exp4` 拆成两个子演示，分别对应 Channel 技巧和 `RWMutex`。
- `exp5` 聚焦 `sync.Pool` 的性能优化演示，不额外引入数据库连接池代码。
- `exp6` 使用真实 Redis 与 PostgreSQL 演示分层存储架构。
- 当前目录保持原路径不变，避免影响既有讲义、截图和运行命令。
