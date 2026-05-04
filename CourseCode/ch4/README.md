# 英雄集结演示实验 —— 代码与演示说明

> 本目录 (`ch4/`) 是一个**独立的 Go module**，围绕 Go 并发控制、锁机制、对象池优化和分层存储架构，提供 6 组可运行演示程序。
> 实验一至五为本地或本机网络演示；实验六同时提供“纯内存简化版”和“Redis + PostgreSQL 完整版”，便于先讲架构分层，再讲真实基础设施。

---

## 目录结构

```text
ch4/
├── go.mod                              # 独立模块 ch4
├── go.sum                              # 依赖校验文件
├── README.md                           # ← 本文件
├── 英雄集结演示实验.md                  # 实验目标与讲解要点
├── internal/
│   ├── exp6simple/demo.go              # 实验六简化版：纯内存持久层/缓存层演示
│   └── exp6demo/demo.go                # 实验六共享：Redis/PostgreSQL 初始化与数据操作
└── cmd/
    ├── README.md                       # cmd 子目录索引
    ├── exp1/README.md                       # 实验一：服务器主循环解耦
    ├── exp1/01_basic_serial_blocking_demo/  # 实验一：串行阻塞最小原理
    ├── exp1/02_network_serial_warzone/      # 实验一：串行收包反例
    ├── exp1/03_basic_goroutine_receiver_demo/ # 实验一：goroutine 收包最小原理
    ├── exp1/04_network_goroutine_warzone/   # 实验一：goroutine 收包嵌入版
    ├── exp1/05_basic_event_driven_demo/     # 实验一：事件驱动最小原理
    ├── exp1/06_network_event_driven_warzone/ # 实验一：事件驱动 + 增量同步
    ├── exp2/wrong/                     # 实验二：临界区与互斥锁（错误版）
    ├── exp2/right/                     # 实验二：临界区与互斥锁（修复版）
    ├── exp3/busy_wait/                 # 实验三：条件等待与唤醒（忙等版）
    ├── exp3/cond_wait/                 # 实验三：条件等待与唤醒（Cond 版）
    ├── exp3/spurious_wakeup_demo/      # 实验三：条件等待与唤醒（虚假唤醒）
    ├── exp4/channel_semaphore/         # 实验四：锁策略与并发粒度（Channel 信号量）
    ├── exp4/channel_timeout_lock/      # 实验四：锁策略与并发粒度（Channel 超时锁）
    ├── exp4/lock_cost_demo/            # 实验四：锁策略与并发粒度（锁代价）
    ├── exp4/rw_mutex/                  # 实验四：锁策略与并发粒度（RWMutex）
    ├── exp5/sync_pool_demo/            # 实验五：对象复用与性能观测（sync.Pool）
    ├── exp5/connection_pool_demo/      # 实验五：对象复用与性能观测（连接池）
    │   ├── server/
    │   └── client/
    ├── exp5/perf_observe_demo/         # 实验五：对象复用与性能观测（pprof）
    ├── exp6/write_through_simple_demo/ # 实验六：缓存与分层存储（Write Through 简化版）
    ├── exp6/cache_aside_simple_demo/   # 实验六：缓存与分层存储（Cache Aside 简化版）
    ├── exp6/write_through_demo/        # 实验六：缓存与分层存储（Write Through）
    └── exp6/cache_aside_demo/          # 实验六：缓存与分层存储（Cache Aside）
```

---

## 环境搭建

### 前提条件

| 依赖 | 版本 | 说明 |
|------|------|------|
| Go | >= 1.21 | 所有实验使用 Go 1.21+ |
| Docker Desktop | 已安装并启动 | 仅实验 6 完整版需要 |
| Redis | 7 | 建议使用 Docker 容器 `ch4-redis`，仅实验 6 完整版需要 |
| PostgreSQL | 16 | 建议使用 Docker 容器 `ch4-postgres`，仅实验 6 完整版需要 |

### 首次构建

```powershell
# 进入 ch4 目录
cd ch4

# 首次拉取代码后，先整理并拉取依赖
go mod tidy

# 编译全部实验，验证无错误
go build ./...
```

---

## 实验六环境准备

实验六现在有两组入口：

- 简化版：纯内存模拟持久层和缓存层，不需要 Docker、Redis、PostgreSQL。
- 完整版：连接真实 Redis 和 PostgreSQL，用来观察真实中间件下的读写路径与一致性。

如果你这节课只想先讲“为什么要分层、冷热数据怎么放、访问路径为什么不同”，直接运行简化版即可：

```powershell
go run ./cmd/exp6/write_through_simple_demo
go run ./cmd/exp6/cache_aside_simple_demo
```

下面这部分环境准备只针对实验六的完整版。

### 1. 启动 Docker Desktop

```powershell
docker ps
```

如果能正常返回容器列表，说明 Docker 已经可用。

### 2. 启动 Redis 容器

第一次创建并启动：

```powershell
docker run -d --name ch4-redis -p 6379:6379 redis:7
```

如果容器已经创建过，直接启动：

```powershell
docker start ch4-redis
```

检查 Redis：

```powershell
docker exec -it ch4-redis redis-cli ping
```

预期输出：

```text
PONG
```

### 3. 启动 PostgreSQL 容器

第一次创建并启动：

```powershell
docker run -d --name ch4-postgres -e POSTGRES_USER=你的用户名 -e POSTGRES_PASSWORD=你的密码 -e POSTGRES_DB=postgres -p 5432:5432 postgres:16
```

如果容器已经创建过，直接启动：

```powershell
docker start ch4-postgres
```

对应连接串格式：

```text
postgres://你的用户名:你的密码@127.0.0.1:5432/postgres?sslmode=disable
```

### 4. 设置环境变量

Windows PowerShell：

```powershell
$env:REDIS_ADDR="127.0.0.1:6379"
$env:PG_DSN="postgres://你的用户名:你的密码@127.0.0.1:5432/postgres?sslmode=disable"
```

macOS / Linux：

```bash
export REDIS_ADDR="127.0.0.1:6379"
export PG_DSN="postgres://你的用户名:你的密码@127.0.0.1:5432/postgres?sslmode=disable"
```

说明：

- `REDIS_ADDR` 指向 Docker 映射到本机的 Redis 地址；默认值是 `127.0.0.1:6379`。
- `PG_DSN` 指向 Docker 映射到本机的 PostgreSQL 地址；代码不会内置个人账号密码，必须由使用者自行设置。

### 5. 手动查库命令

实验六运行前后，可以用以下命令确认 Redis / PostgreSQL 中的数据变化。

```powershell
# Redis：查看金币缓存
docker exec -it ch4-redis redis-cli GET gold:player_1

# Redis：查看配置缓存
docker exec -it ch4-redis redis-cli GET cfg:drop_rate

# PostgreSQL：查看玩家金币
docker exec -it ch4-postgres psql -U 你的用户名 -d postgres -c "SELECT user_id, gold FROM players;"

# PostgreSQL：查看游戏配置
docker exec -it ch4-postgres psql -U 你的用户名 -d postgres -c "SELECT config_key, config_value FROM game_configs;"
```

---

## 各实验演示操作

### 实验一：服务器主循环解耦

**对应页码**：第 4-6, 11-12, 15 页

**知识点**：串行阻塞、goroutine 并发解耦、事件驱动、增量同步。

**实验功能**：先用最小代码讲清楚“串行等待、goroutine 解耦、事件驱动”的因果关系，再把同一个写法嵌入 Warzone 的极简游戏场景。游戏嵌入版压缩为“玩家 ACTION -> 权威 PlayerState -> STATE_UPDATE”，只保留 `fast`（疾风游侠）和 `slow`（断流骑士）两名玩家，便于观察慢连接如何影响或不影响快玩家。

**详细讲解**：见 `cmd/exp1/README.md`，其中标明了每个知识点对应哪一块代码。

#### 演示 1：串行阻塞

```powershell
# 第一步：最小原理演示
go run ./cmd/exp1/01_basic_serial_blocking_demo

# 终端1：服务端
go run ./cmd/exp1/02_network_serial_warzone/server

# 终端2：快玩家
go run ./cmd/exp1/02_network_serial_warzone/client -player fast

# 终端3：慢玩家
go run ./cmd/exp1/02_network_serial_warzone/client -player slow
```

**观察点**：

- 最小演示中，fast 本身只需要 20ms，但主循环先调用 slow 的读取逻辑，所以 fast 也被拖慢。
- 游戏嵌入版中，服务端 `runFrameSerial` 先读 `slow` 的 ACTION 再读 `fast` 的 ACTION，复现“慢玩家拖住后面的玩家输入”。

#### 演示 2：goroutine 并发收包

```powershell
# 第一步：最小原理演示
go run ./cmd/exp1/03_basic_goroutine_receiver_demo

# 终端1：服务端
go run ./cmd/exp1/04_network_goroutine_warzone/server

# 终端2：快玩家
go run ./cmd/exp1/04_network_goroutine_warzone/client -player fast

# 终端3：慢玩家
go run ./cmd/exp1/04_network_goroutine_warzone/client -player slow
```

**观察点**：主循环很快完成收包任务分发；slow 仍会晚到，但被隔离在自己的连接 goroutine 中，不再阻止 fast 的 ACTION 先被应用到权威世界状态。

#### 演示 3：事件驱动 + 增量同步

```powershell
# 第一步：最小原理演示
go run ./cmd/exp1/05_basic_event_driven_demo

# 终端1：服务端
go run ./cmd/exp1/06_network_event_driven_warzone/server

# 终端2：快玩家
go run ./cmd/exp1/06_network_event_driven_warzone/client -player fast

# 终端3：慢玩家
go run ./cmd/exp1/06_network_event_driven_warzone/client -player slow
```

**观察点**：服务器按 tick 前进，只处理已经到达的 ACTION；没有 ACTION 时不等待；增量 `STATE_UPDATE` 只同步发生变化的玩家。

---

### 实验二：临界区与互斥锁

**对应页码**：第 17-21 页

**知识点**：Race Condition、临界区、`sync.Mutex`。

**实验功能**：模拟 5 名玩家同时抢同一个 NPC 的唯一掉落，复现“检查是否可拿”和“标记已拿走”没有放进同一个临界区时产生的重复掉落，再用 Mutex 修复。

```powershell
# 无锁错误版
go run ./cmd/exp2/wrong

# Mutex 修复版
go run ./cmd/exp2/right
```

**操作**：默认每轮会等待回车；如果想自动连续演示，可加 `-auto`。

```powershell
go run ./cmd/exp2/wrong -rounds 3 -auto
go run ./cmd/exp2/right -rounds 3 -auto
```

**观察点**：

- 无锁版可能出现多个赢家，并高亮“核心资源：唯一宝物归属”。
- Mutex 版只允许一个玩家成功领取，其余玩家失败。

**成功标准**：错误版能复现唯一物品被复制；修复版成功领取人数始终为 1。

---

### 实验三：条件等待与唤醒

**对应页码**：第 23-26, 42 页

**知识点**：忙等、`sync.Cond`、等待队列、虚假唤醒。

**实验功能**：在 NPC 宝物为空时，对比“玩家不断 Lock -> Check -> Unlock 空转”和“玩家进入 Cond 等待队列，补货后被唤醒”的差别。

#### 演示 1：忙等错误版

```powershell
go run ./cmd/exp3/busy_wait
```

**操作命令**：

```text
status
restock 3
quit
```

**观察点**：补货前执行 `status`，可以看到玩家已经累计大量白跑次数。

#### 演示 2：Cond 等待唤醒版

```powershell
go run ./cmd/exp3/cond_wait
```

**操作命令**：

```text
status
restock 3
quit
```

**观察点**：库存为空时玩家进入等待队列；补货后被 `Signal()` 唤醒，并在 `for npc.Items == 0` 中重新检查条件。

#### 演示 3：虚假唤醒极简版

```powershell
go run ./cmd/exp3/spurious_wakeup_demo
```

**观察点**：错误版用 `if`，被一次“没有补货的通知”唤醒后就继续取空队列；正确版用 `for`，每次 `Wait()` 返回后都重新检查队列是否真的有宝物。

---

### 实验四：锁策略与并发粒度

**对应页码**：第 45-46, 49-51 页

**知识点**：Channel 信号量、超时锁、`sync.RWMutex`。

**实验功能**：演示 Go 中不止有普通 Mutex，还可以用 Channel 表达许可数和超时竞争，并用 RWMutex 优化读多写少场景；同时用一个极简性能对照感受“锁加多了程序会变慢”。

#### 演示 1：锁的代价极简版

```powershell
go run ./cmd/exp4/lock_cost_demo
```

**观察点**：同样计算最终金币，`每次击杀都 Lock/Unlock` 会比 `本地累计后再合并一次` 慢很多；如果课堂机器太快，可以加大 `-ops`。

```powershell
go run ./cmd/exp4/lock_cost_demo -workers 8 -ops 2000000
```

#### 演示 2：Channel 信号量

```powershell
go run ./cmd/exp4/channel_semaphore
```

**操作命令**：

```text
wave 6
status
wait
quit
```

**观察点**：Channel 容量为 3，同一时刻真正进入执行区的 Worker 数量不会超过 3。

#### 演示 3：Channel 超时锁

```powershell
go run ./cmd/exp4/channel_timeout_lock
```

**观察点**：玩家 A 占用资源 5 秒，玩家 B 只等待 2 秒；超时后进入降级逻辑，避免永久阻塞。

#### 演示 4：Mutex 与 RWMutex 对照

```powershell
go run ./cmd/exp4/rw_mutex
```

**操作**：按回车运行普通 Mutex 对照，再按回车运行 RWMutex 对照。

**观察点**：

- 普通 Mutex 下首批读者也会串行。
- RWMutex 下首批读者可以并发读取。
- 写者排队后，后续读者仍会等待，避免写者长期饥饿。

---

### 实验五：对象复用与性能观测

**对应页码**：第 54, 56, 58 页

**知识点**：`sync.Pool`、对象复用、连接池、长连接复用、`pprof`、Benchmark、GC 压力、尾延迟。

**实验功能**：先对比高频请求中“每次 new 一个 10KB 临时缓冲”和“通过 sync.Pool 复用缓冲”的分配次数、GC 次数和尾延迟；再用本机 TCP server 演示“每个请求都新建短连接”和“从连接池借还长连接”的耗时差异；最后用 pprof 和 benchmark 演示如何定位 CPU 热点、内存分配、锁竞争和 goroutine 泄漏。

#### 演示 1：对象池 sync.Pool

```powershell
# 优化前：每次 new
go run ./cmd/exp5/sync_pool_demo before -requests 12000 -payload-kb 10 -work 500 -concurrency 32

# 优化后：sync.Pool 复用
go run ./cmd/exp5/sync_pool_demo after -requests 12000 -payload-kb 10 -work 500 -concurrency 32
```

**观察点**：

- `Mallocs` 是否明显下降。
- `GC` 次数是否明显下降。
- `P99` / `P99.9` 是否降低。

**说明**：这里演示的是对象池，不是数据库连接池；两者都叫“池”，但资源类型和生命周期约束不同。

#### 演示 2：网络连接池（两个终端）

```powershell
# 终端1：启动模拟游戏网关服务端
go run ./cmd/exp5/connection_pool_demo/server

# 终端2：运行短连接 / 连接池对比客户端
go run ./cmd/exp5/connection_pool_demo/client
```

**观察点**：短连接版本每个请求都 `Dial + Close`；连接池版本只创建少量长连接，后续请求从池子里借连接、用完再还。输出会直接对比建连次数和总耗时。

如果课堂机器太快，可以放大模拟建连成本：

```powershell
go run ./cmd/exp5/connection_pool_demo/server -handshake-ms 50
go run ./cmd/exp5/connection_pool_demo/client -requests 240 -concurrency 24 -max-open 16 -max-idle 8 -lifetime-ms 200
```

**贴近 PPT 的看法**：这个 demo 现在明确对应连接池初始化三项配置：

- `maxOpenConns`：最多同时打开多少条连接
- `maxIdleConns`：最多保留多少条空闲连接
- `connMaxLifetime`：连接活多久后需要重建

#### 演示 3：性能观测工具教程

详细安装和命令见：

```powershell
CourseCode/ch4/cmd/exp5/perf_observe_demo/README.md
```

最小命令示例：

```powershell
# Benchmark + 分配观测
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench . -benchmem

# CPU profile
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkCPUHotspotBad -benchtime 2s -cpuprofile cpu_bad.prof
go tool pprof -top cpu_bad.prof

# 网页端建议写法
go tool pprof -http=:8081 cpu_bad.prof

# goroutine profile HTTP 观测
go run ./cmd/exp5/perf_observe_demo -mode leak -seconds 20
```

**观察点**：用 `BenchmarkCPUHotspotBad/Good` 对比 CPU 热点优化；用 `BenchmarkHeapAllocBad/Good` 对比 `allocs/op`；用 `BenchmarkMutexContentionBad/Good` 和 `-mutexprofile` 观察锁竞争；用 `-mode leak/fixed` 观察 goroutine 数量是否持续上涨。做 goroutine live 观测时，要先看到第一个终端打印 `[pprof] HTTP 服务已启动`，再去第二个终端执行 `go tool pprof`。

---

### 实验六：缓存与分层存储

**对应页码**：第 61-63 页

**知识点**：分层存储、冷热数据、Write Through、Cache Aside。

**实验功能**：先用纯内存简化版讲清楚“持久层 + 缓存层”的职责分工，再用真实 Redis 与 PostgreSQL 演示完整分层存储。核心资产用 Write Through 保证双写顺序；配置类数据用 Cache Aside 展示缓存未命中、回填、失效与再次命中。

#### 演示 0：简化版（推荐先讲）

```powershell
go run ./cmd/exp6/write_through_simple_demo
go run ./cmd/exp6/cache_aside_simple_demo
```

**观察点**：不依赖任何容器，只用内存 map 模拟“持久层 / 缓存层”两层结构，先让学生看懂冷热数据放在哪里、读写路径为什么不同。

#### 演示 1：Write Through（完整版）

```powershell
go run ./cmd/exp6/write_through_demo
```

**观察点**：扣除 `player_1` 金币时，先更新 PostgreSQL，再同步 Redis；最后输出 PostgreSQL 与 Redis 的金币一致性检查。

**手动验证**：

```powershell
docker exec -it ch4-postgres psql -U 你的用户名 -d postgres -c "SELECT user_id, gold FROM players;"
docker exec -it ch4-redis redis-cli GET gold:player_1
```

#### 演示 2：Cache Aside（完整版）

```powershell
go run ./cmd/exp6/cache_aside_demo
```

**观察点**：第一次读配置时缓存未命中并回填；第二次读命中 Redis；更新配置时删除缓存；再次读取时重新从 PostgreSQL 回填。

**手动验证**：

```powershell
docker exec -it ch4-postgres psql -U 你的用户名 -d postgres -c "SELECT config_key, config_value FROM game_configs;"
docker exec -it ch4-redis redis-cli GET cfg:drop_rate
```

---

## 各实验资源与依赖对照

| 实验 | 额外依赖 | 说明 |
|------|----------|------|
| 实验一 | 无 | 本机网络并发演示 |
| 实验二 | 无 | 单机并发演示 |
| 实验三 | 无 | 单机并发演示 |
| 实验四 | 无 | 单机并发演示 |
| 实验五 | 无 | 单机性能演示 |
| 实验六 | 简化版无；完整版需要 Redis + PostgreSQL | 建议通过 Docker 容器启动完整版 |

---

## 核心知识点对照表

| 实验 | 演示目标 | 关键结构/机制 |
|------|----------|----------------|
| 实验一 | 单线程阻塞 vs 并发解耦 | goroutine、事件驱动、tick、增量同步 |
| 实验二 | 数据竞争复现与修复 | `sync.Mutex` |
| 实验三 | 忙等替换为阻塞等待 | `sync.Cond`、`Wait`、`Signal` |
| 实验四 | 锁策略与并发粒度 | Channel 信号量、超时锁、`sync.RWMutex` |
| 实验五 | 临时对象复用 | `sync.Pool`、GC、尾延迟 |
| 实验六 | 分层存储与缓存一致性 | Redis、PostgreSQL、Write Through、Cache Aside |
