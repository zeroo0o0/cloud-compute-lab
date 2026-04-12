# ch4/cmd 目录索引

本目录仅做结构整理说明，不改动任何程序代码与运行路径。

---

## 目录总览

```text
cmd/
├─ exp1/
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
   ├─ write_through_demo/
   └─ cache_aside_demo/
```

---

## 运行入口

- exp1：突破单线程瓶颈
  - `go run ./cmd/exp1/network_serial_server_demo server`
  - `go run ./cmd/exp1/network_serial_server_demo client -player 1`
  - `go run ./cmd/exp1/network_goroutine_server_demo server`
  - `go run ./cmd/exp1/network_goroutine_server_demo client -player 1`
  - `go run ./cmd/exp1/network_event_driven_sync_demo server`
  - `go run ./cmd/exp1/network_event_driven_sync_demo client -player 1`
  - 说明：`network_serial_server_demo` 展示慢连接如何拖慢串行收包；`network_goroutine_server_demo` 展示每条连接独立 goroutine 收包；`network_event_driven_sync_demo` 展示按 tick 处理已到达事件并做增量同步。

- exp2：临界区与数据竞争
  - `go run ./cmd/exp2/wrong`
  - `go run ./cmd/exp2/right`
  - 说明：`wrong` 复现多个玩家抢同一个 NPC 唯一掉落时的重复领取；`right` 用 `sync.Mutex` 保护“检查 + 修改”临界区。

- exp3：告别忙等
  - `go run ./cmd/exp3/busy_wait`
  - `go run ./cmd/exp3/cond_wait`
  - 说明：`busy_wait` 展示库存为空时玩家不断空转检查；`cond_wait` 展示玩家进入 `sync.Cond` 等待队列并在补货后被唤醒。

- exp4：锁的进阶技巧与粒度优化
  - `go run ./cmd/exp4/channel_semaphore`
  - `go run ./cmd/exp4/channel_timeout_lock`
  - `go run ./cmd/exp4/rw_mutex`
  - 说明：`channel_semaphore` 用 Channel 容量限制最大并发；`channel_timeout_lock` 用 `select + time.After` 演示超时降级；`rw_mutex` 对比普通 Mutex 与 RWMutex 在读多写少场景下的等待差异。

- exp5：高并发性能榨取
  - `go run ./cmd/exp5/sync_pool_demo before`
  - `go run ./cmd/exp5/sync_pool_demo after`
  - 说明：`before` 每次请求都新建临时 `bytes.Buffer`；`after` 使用 `sync.Pool` 复用临时对象。两组使用相同请求数、缓冲大小、工作量和并发度，便于对比分配次数、GC 次数和尾延迟。

- exp6：游戏数据分层存储架构
  - `go run ./cmd/exp6/write_through_demo`
  - `go run ./cmd/exp6/cache_aside_demo`
  - 说明：`write_through_demo` 演示金币扣减时先写 PostgreSQL 再同步 Redis；`cache_aside_demo` 演示配置数据的缓存未命中、回填、失效与再次命中。运行前需要 Redis、PostgreSQL 和 `PG_DSN` 环境变量。

---

## 命名说明

- `exp1` 包含 3 个网络演示入口，均按“1 个 server + 4 个 client”运行。
- `exp2` 和 `exp3` 使用“错误版 / 修复版”结构，便于课堂前后对照。
- `exp4` 将 Channel 信号量、Channel 超时锁、RWMutex 对照拆成 3 个独立入口。
- `exp5` 将对象池优化拆成 `before` 和 `after` 两个模式，便于单独记录结果。
- `exp6` 将 `Write Through` 和 `Cache Aside` 拆成两个独立程序，避免两种一致性方案混在同一次输出中。
- 当前目录保持原路径不变，避免影响既有讲义、截图和运行命令。
