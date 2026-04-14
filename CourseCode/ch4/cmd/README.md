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

- `exp1` 包含 3 个最小原理演示和 3 组网络嵌入演示；网络嵌入版均按“1 个 server + 2 个 client（fast/slow）”运行。
- `exp2` 和 `exp3` 使用“错误版 / 修复版”结构，便于课堂前后对照。
- `exp4` 将 Channel 信号量、Channel 超时锁、RWMutex 对照拆成 3 个独立入口。
- `exp5` 将对象池优化拆成 `before` 和 `after` 两个模式，便于单独记录结果。
- `exp6` 将 `Write Through` 和 `Cache Aside` 拆成两个独立程序，避免两种一致性方案混在同一次输出中。
- exp1 已按教学需要拆分 server/client 入口；旧的 `network_*_demo server/client` 混合入口不再使用。
