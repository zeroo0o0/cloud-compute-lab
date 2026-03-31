# 英雄集结演示实验 —— 代码与演示说明

> 本目录 (`ch4/`) 是一个**独立的 Go module**，围绕 Go 并发与后端架构设计，提供 6 个可运行的演示实验。
> 其中实验 1~5 为单机并发教学演示；实验 6 使用 Docker 中的 Redis 与 Docker 中的 PostgreSQL，演示分层存储架构。

---

## 目录结构

```text
ch4/
├── go.mod                              # 独立模块 ch4
├── go.sum                              # 依赖校验文件
├── README.md                           # ← 本文件
├── 英雄集结演示实验.md                  # 实验要求与讲解要点
└── cmd/
    ├── README.md                       # cmd 子目录说明
    ├── exp1/local_serial_loop_demo/    # ① 本地串行主循环
    ├── exp1/network_serial_server_demo/# ① 网络版：服务器串行收包
    ├── exp1/network_goroutine_server_demo/ # ① 网络版：服务器独立线程收包
    ├── exp1/network_event_driven_sync_demo/ # ① 网络版：事件驱动 + 增量同步
    ├── exp2/wrong/                     # ② 无锁竞态错误版
    ├── exp2/right/                     # ② Mutex 修复版
    ├── exp3/busy_wait/                 # ③ 忙等错误版
    ├── exp3/cond_wait/                 # ③ Cond 修复版
    ├── exp4/channel_semaphore/         # ④ Channel 信号量
    ├── exp4/channel_timeout_lock/      # ④ Channel 超时锁
    ├── exp4/rw_mutex/                  # ④ RWMutex 读写分离
    ├── exp5/sync_pool_demo/            # ⑤ sync.Pool 性能优化
    └── exp6/storage_arch/              # ⑥ Redis + PostgreSQL 分层存储
```

---

## 环境搭建

### 前提条件

| 依赖 | 版本 | 说明 |
|------|------|------|
| Go | ≥ 1.21 | 所有实验使用 Go 1.21+ |
| Docker Desktop | 已安装并启动 | 仅实验 6 需要，用于启动 Redis 与 PostgreSQL 容器 |

### 首次构建

```powershell
# 进入 ch4 目录
cd ch4

# 验证实验 1~6
go build ./cmd/exp1/...
go build ./cmd/exp2/...
go build ./cmd/exp3/...
go build ./cmd/exp4/...
go build ./cmd/exp5/...
go build ./cmd/exp6/...
```



### 实验六运行前准备

实验 6 不是纯本地模拟，它依赖两个真实环境，并且这两个环境现在都通过 Docker Desktop 提供：

- Redis：Docker 容器 `ch4-redis`
- PostgreSQL：Docker 容器 `ch4-postgres`

#### 1. 启动 Docker Desktop

先确认 Docker Desktop 已经启动；如果 Docker 没启动，下面的 `docker run` 和 `docker start` 都会失败。

检查命令：

```powershell
docker ps
```

如果能正常返回容器列表，说明 Docker 已经可用。

#### 2. 启动 Redis 容器

第一次创建并启动：

```powershell
docker run -d --name ch4-redis -p 6379:6379 redis:7
```

如果容器已经创建过，直接启动：

```powershell
docker start ch4-redis
```

检查 Redis 是否正常：

```powershell
docker exec -it ch4-redis redis-cli ping
```

预期输出：

```text
PONG
```

#### 3. 启动 PostgreSQL 容器

第一次创建并启动：

```powershell
docker run -d --name ch4-postgres -e POSTGRES_USER=你的用户名 -e POSTGRES_PASSWORD=你的密码 -e POSTGRES_DB=postgres -p 5432:5432 postgres:16
```

如果容器已经创建过，直接启动：

```powershell
docker start ch4-postgres
```

这套配置对应的 PostgreSQL 连接串是：

```text
postgres://你的用户名:你的密码@127.0.0.1:5432/postgres?sslmode=disable
```

#### 4. 设置实验六环境变量

在 PowerShell 中设置：

```powershell
$env:REDIS_ADDR="127.0.0.1:6379"
$env:PG_DSN="postgres://你的用户名:你的密码@127.0.0.1:5432/postgres?sslmode=disable"
```

其中：

- `REDIS_ADDR` 指向 Docker 中映射到本机 `6379` 的 Redis
- `PG_DSN` 指向 Docker 中映射到本机 `5432` 的 PostgreSQL
- 代码不会内置任何个人 PostgreSQL 账号密码，使用前请由每位使用者自行设置 `PG_DSN`

#### 5. 运行实验六

```powershell
go run ./cmd/exp6/storage_arch
```

程序启动后会自动完成这些动作：

- 连接 Redis
- 连接 PostgreSQL
- 自动创建 `players` 与 `game_configs` 两张表
- 自动写入演示初始数据
- 依次演示 `Write Through` 和 `Cache Aside`

#### 6. 常见失败原因

- `docker` 命令报错：通常是 Docker Desktop 没有启动。
- Redis 连接失败：通常是 `ch4-redis` 容器没启动，或 `6379` 端口未映射成功。
- PostgreSQL 连接失败：通常是 `ch4-postgres` 容器没启动，或 `PG_DSN` 与容器账号密码不一致。

> 实验 6 依赖真实 Redis 与真实 PostgreSQL；实验 1~5 不需要额外环境。

---

## 各步骤演示操作

### Step 1 — 突破单线程瓶颈

**知识点**：串行阻塞、Goroutine 解耦、事件驱动、增量同步。

```powershell
go run ./cmd/exp1/local_serial_loop_demo
go run ./cmd/exp1/network_serial_server_demo server
go run ./cmd/exp1/network_serial_server_demo client
go run ./cmd/exp1/network_goroutine_server_demo server
go run ./cmd/exp1/network_goroutine_server_demo client
go run ./cmd/exp1/network_event_driven_sync_demo server
go run ./cmd/exp1/network_event_driven_sync_demo client
```

**观察点**：

- `local_serial_loop_demo` 展示最原始的本地串行主循环，慢玩家会直接拖慢整帧。
- `network_serial_server_demo` 用 2 个终端模拟真实客户端/服务器，但服务器仍按连接顺序串行收包。
- `network_goroutine_server_demo` 保持真实网络分离，同时把每条连接的收包交给独立 goroutine，输出风格与串行网络版保持一致，方便直接对比。
- `network_event_driven_sync_demo` 进一步展示真实网络下的事件驱动与增量同步：服务器按 tick 前进，只处理已经到达的输入，不等最慢玩家。

**课堂结论**：

- 先看 `local_serial_loop_demo`，能理解“慢输入会拖帧”这个最基本现象。
- 再看 `network_serial_server_demo`，能看清问题不只是“玩家慢”，而是服务器在串行等待某条慢连接。
- 切到 `network_goroutine_server_demo` 后，慢连接仍然存在，但不会再让服务器主循环卡死在某一条连接上。
- 最后看 `network_event_driven_sync_demo`，能补齐“只消费已到达事件、只同步变化状态”的思路。

---

### Step 2 — 临界区与数据竞争

**知识点**：Race Condition、临界区、`sync.Mutex`。

```powershell
go run ./cmd/exp2/wrong
go run ./cmd/exp2/right
```

**观察点**：

- 无锁版会出现“唯一物品被多次领取”，而且重复领取人数不必每轮都一样。
- 加锁版只允许一个玩家成功获得宝物。
- 对照输出可以看到临界区保护前后的差异。

**课堂结论**：

- 业务规则写得再正确，只要“检查是否可拿”和“标记已拿走”不在同一个临界区里，就仍然会出现竞态窗口。
- `sync.Mutex` 修复的不是“谁先抢到”的业务逻辑，而是把检查与修改变成原子的一段。

---

### Step 3 — 告别忙等

**知识点**：忙等、`sync.Cond`、等待队列、虚假唤醒。

```powershell
go run ./cmd/exp3/busy_wait
go run ./cmd/exp3/cond_wait
```

**观察点**：

- 忙等版会出现极高的空转次数。
- `sync.Cond` 版会展示等待、唤醒和重新检查条件。
- `for` 循环重检逻辑可用于讲解“为什么不能只用 `if`”。

**课堂结论**：

- 忙等的问题不是功能错误，而是没有库存时线程仍在不停抢锁和检查，白白消耗 CPU。
- `Cond.Wait()` 会先释放锁再休眠，被唤醒后重新抢锁，所以必须用 `for` 重检条件来防住虚假唤醒。

---

### Step 4 — 锁的进阶技巧与粒度优化


**知识点**：Channel 信号量、超时锁、`sync.RWMutex`。

```powershell
go run ./cmd/exp4/channel_semaphore
go run ./cmd/exp4/channel_timeout_lock
go run ./cmd/exp4/rw_mutex
```

**观察点**：

- `channel_semaphore` 展示如何用 Channel 控制最大并发数。
- `channel_timeout_lock` 展示等待超时后如何快速降级，不把协程永远卡死。
- `rw_mutex` 除了展示读多写少场景下的并发读取优势，也显式展示“写者排队后”和“写入进行中”到来的读者都会被阻塞。
- 这一组实验用于对比“锁不仅只有 Mutex 一种用法”。

**课堂结论**：

- Channel 不只适合传消息，也很适合表达“许可数”和“带超时的竞争”。
- `RWMutex` 的重点不只是“多个读者可以并发”，还包括一旦写者开始排队，后续读者也要等待，避免写者长期饥饿。

---

### Step 5 — 高并发性能榨取

**知识点**：`sync.Pool`、对象复用、GC 压力、尾延迟。

```powershell
go run ./cmd/exp5/sync_pool_demo
```

**观察点**：

- 对比 `new(bytes.Buffer)` 与 `sync.Pool` 的总耗时。
- 观察分配次数、GC 次数或尾延迟差异。
- 理解“池化不是为了炫技，而是为了减少重复分配”。

**课堂结论**：

- `sync.Pool` 不保证每次都命中，但在高频临时对象场景里，通常能明显减少分配与 GC 压力。
- 这里演示的是对象池，不是数据库连接池；两者都叫“池”，但适用资源类型和约束不同。

---

### Step 6 — 游戏数据分层存储架构

**知识点**：Redis、PostgreSQL、Write Through、Cache Aside。

**运行前环境要求**：

- Docker Desktop 已启动
- Redis 容器 `ch4-redis` 已运行并映射 `6379:6379`
- PostgreSQL 容器 `ch4-postgres` 已运行并映射 `5432:5432`
- PowerShell 中已设置 `PG_DSN`， $env:PG_DSN="postgres://账号:密码@127.0.0.1:5432/postgres?sslmode=disable"

**运行命令**：

```powershell
go run ./cmd/exp6/storage_arch
```

**观察点**：

- `Write Through`：先写 PostgreSQL，再同步 Redis。
- `Cache Aside`：先查 Redis，未命中时再查 PostgreSQL 并回填。
- 终端会输出中文日志，便于课堂中逐步讲解数据流。
- 程序会自动初始化表结构与演示数据，适合直接投屏讲解。

---

## 各步骤资源与依赖对照

| 步骤 | 额外依赖 | 说明 |
|------|----------|------|
| 1 | 无 | 单机并发演示 |
| 2 | 无 | 单机并发演示 |
| 3 | 无 | 单机并发演示 |
| 4 | 无 | 单机并发演示 |
| 5 | 无 | 单机性能演示 |
| 6 | Redis + PostgreSQL | Redis 与 PostgreSQL 都建议使用 Docker 容器启动 |

---

## 核心知识点对照表

| 步骤 | 演示目标 | 关键结构/机制 |
|------|----------|----------------|
| 1 | 单线程阻塞 vs 并发解耦 | `goroutine`、事件驱动 |
| 2 | 竞态条件复现与修复 | `sync.Mutex` |
| 3 | 忙等替换为阻塞等待 | `sync.Cond` |
| 4 | 锁的高级技巧 | Channel、超时锁、`sync.RWMutex` |
| 5 | 高并发性能优化 | `sync.Pool`、GC、尾延迟 |
| 6 | 分层存储架构 | Redis、PostgreSQL、Write Through、Cache Aside |
