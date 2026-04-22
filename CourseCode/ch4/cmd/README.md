# ch4/cmd 目录索引

本目录仅做结构整理说明，不改动任何程序代码与运行路径。

---

## 目录总览

```text
cmd/
├─ exp1/
│  ├─ README.md
│  ├─ 01_basic_serial_blocking_demo/
│  ├─ 02_network_serial_warzone/
│  ├─ 03_basic_goroutine_receiver_demo/
│  ├─ 04_network_goroutine_warzone/
│  ├─ 05_basic_event_driven_demo/
│  └─ 06_network_event_driven_warzone/
├─ exp2/
│  ├─ wrong/
│  └─ right/
├─ exp3/
│  ├─ busy_wait/
│  ├─ cond_wait/
│  └─ spurious_wakeup_demo/
├─ exp4/
│  ├─ channel_semaphore/
│  ├─ channel_timeout_lock/
│  ├─ lock_cost_demo/
│  └─ rw_mutex/
├─ exp5/
│  ├─ sync_pool_demo/
│  ├─ connection_pool_demo/
│  └─ perf_observe_demo/
└─ exp6/
   ├─ write_through_simple_demo/
   ├─ cache_aside_simple_demo/
   ├─ write_through_demo/
   └─ cache_aside_demo/
```

---

## 运行入口

- exp1：突破单线程瓶颈
  - `go run ./cmd/exp1/01_basic_serial_blocking_demo`
  - `go run ./cmd/exp1/02_network_serial_warzone/server`
  - `go run ./cmd/exp1/02_network_serial_warzone/client -player fast`
  - `go run ./cmd/exp1/02_network_serial_warzone/client -player slow`
  - `go run ./cmd/exp1/03_basic_goroutine_receiver_demo`
  - `go run ./cmd/exp1/04_network_goroutine_warzone/server`
  - `go run ./cmd/exp1/04_network_goroutine_warzone/client -player fast`
  - `go run ./cmd/exp1/04_network_goroutine_warzone/client -player slow`
  - `go run ./cmd/exp1/05_basic_event_driven_demo`
  - `go run ./cmd/exp1/06_network_event_driven_warzone/server`
  - `go run ./cmd/exp1/06_network_event_driven_warzone/client -player fast`
  - `go run ./cmd/exp1/06_network_event_driven_warzone/client -player slow`
  - 说明：`01/03/05_basic_*` 用最小代码讲原理；`02/04/06_network_*_warzone/server|client` 把同一原理嵌入 Warzone 的“玩家 ACTION -> 权威 PlayerState -> STATE_UPDATE”极简流程。服务端和客户端已拆开，具体知识点对应代码见 `cmd/exp1/README.md`。

- exp2：临界区与数据竞争
  - `go run ./cmd/exp2/wrong`
  - `go run ./cmd/exp2/right`
  - 说明：`wrong` 复现多个玩家抢同一个 NPC 唯一掉落时的重复领取；`right` 用 `sync.Mutex` 保护“检查 + 修改”临界区。

- exp3：告别忙等
  - `go run ./cmd/exp3/busy_wait`
  - `go run ./cmd/exp3/cond_wait`
  - `go run ./cmd/exp3/spurious_wakeup_demo`
  - 说明：`busy_wait` 展示库存为空时玩家不断空转检查；`cond_wait` 展示玩家进入 `sync.Cond` 等待队列并在补货后被唤醒；`spurious_wakeup_demo` 用最小队列演示为什么 `Wait()` 后必须用 `for` 重新检查条件，而不是用 `if`。

- exp4：锁的进阶技巧与粒度优化
  - `go run ./cmd/exp4/lock_cost_demo`
  - `go run ./cmd/exp4/channel_semaphore`
  - `go run ./cmd/exp4/channel_timeout_lock`
  - `go run ./cmd/exp4/rw_mutex`
  - 说明：`lock_cost_demo` 用“刷怪金币”演示频繁加锁的真实耗时；`channel_semaphore` 用 Channel 容量限制最大并发；`channel_timeout_lock` 用 `select + time.After` 演示超时降级；`rw_mutex` 对比普通 Mutex 与 RWMutex 在读多写少场景下的等待差异。

- exp5：高并发性能榨取
  - `go run ./cmd/exp5/sync_pool_demo before`
  - `go run ./cmd/exp5/sync_pool_demo after`
  - `go run ./cmd/exp5/connection_pool_demo`
  - `go test ./cmd/exp5/perf_observe_demo -run '^$' -bench . -benchmem`
  - `go run ./cmd/exp5/perf_observe_demo -mode leak`
  - 说明：`before` 每次请求都新建临时 `bytes.Buffer`；`after` 使用 `sync.Pool` 复用临时对象；`connection_pool_demo` 用本机 TCP 长连接池演示短连接改成长连接池后如何减少重复建连时间，并显式展示 `maxOpenConns / maxIdleConns / connMaxLifetime` 三项配置；`perf_observe_demo` 提供 CPU、Heap、Mutex、Goroutine 的 pprof 教程，具体命令见该目录 README。

- exp6：游戏数据分层存储架构
  - `go run ./cmd/exp6/write_through_simple_demo`
  - `go run ./cmd/exp6/cache_aside_simple_demo`
  - `go run ./cmd/exp6/write_through_demo`
  - `go run ./cmd/exp6/cache_aside_demo`
  - 说明：`*_simple_demo` 是纯内存简化版，用 map 模拟持久层与缓存层，先讲清“分层、冷热数据、访问顺序”；`write_through_demo` 和 `cache_aside_demo` 是真实 Redis + PostgreSQL 完整版，运行前需要 Redis、PostgreSQL 和 `PG_DSN` 环境变量。

---

## 命名说明

- `exp1` 包含 3 个最小原理演示和 3 组网络嵌入演示；网络嵌入版均按“1 个 server + 2 个 client（fast/slow）”运行。
- `exp2` 和 `exp3` 使用“错误版 / 修复版”结构，便于课堂前后对照。
- `exp4` 将锁代价、Channel 信号量、Channel 超时锁、RWMutex 对照拆成 4 个独立入口。
- `exp5` 包含对象池、连接池和性能观测工具三个演示；对象池用 `before/after` 模式对比临时对象复用，连接池用本机 TCP server 对比短连接和长连接池，性能观测工具教程覆盖 pprof CPU/Heap/Mutex/Goroutine。
- `exp6` 现在同时提供“简化版”和“完整版”：简化版先讲架构分层，完整版再连接真实 Redis / PostgreSQL；`Write Through` 和 `Cache Aside` 仍然各自拆成独立入口，避免两种一致性方案混在同一次输出中。
- exp1 已按教学需要拆分 server/client 入口；旧的 `network_*_demo server/client` 混合入口不再使用。
