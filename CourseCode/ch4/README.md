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
    ├── exp1/single_thread_demo/        # ① 单线程卡顿传染演示
    ├── exp1/goroutine_demo/            # ① Goroutine 解耦与事件驱动
    ├── exp2/wrong/                     # ② 无锁竞态错误版
    ├── exp2/right/                     # ② Mutex 修复版
    ├── exp3/busy_wait/                 # ③ 忙等错误版
    ├── exp3/cond_wait/                 # ③ Cond 修复版
    ├── exp4/channel_tricks/            # ④ Channel 信号量 + 超时锁
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

# 编译全部（验证无错误）
go build ./...
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
go run ./cmd/exp1/single_thread_demo
go run ./cmd/exp1/goroutine_demo
```

**观察点**：

- 单线程版本中，慢玩家的 `500ms` 延迟会拖慢整帧。
- Goroutine 版本中，主逻辑不再被慢连接拖住。
- 事件驱动部分展示“只在事件到达时推进状态”的思路。

---

### Step 2 — 临界区与数据竞争

**知识点**：Race Condition、临界区、`sync.Mutex`。

```powershell
go run ./cmd/exp2/wrong
go run ./cmd/exp2/right
```

**观察点**：

- 无锁版会出现“唯一物品被多次领取”。
- 加锁版只允许一个玩家成功获得宝物。
- 对照输出可以看到临界区保护前后的差异。

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

---

### Step 4 — 锁的进阶技巧与粒度优化

**知识点**：Channel 信号量、超时锁、`sync.RWMutex`。

```powershell
go run ./cmd/exp4/channel_tricks
go run ./cmd/exp4/rw_mutex
```

**观察点**：

- `channel_tricks` 展示限流与超时降级。
- `rw_mutex` 展示读多写少场景下的并发读取优势。
- 这一组实验用于对比“锁不仅只有 Mutex 一种用法”。

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

---

### Step 6 — 游戏数据分层存储架构

**知识点**：Redis、PostgreSQL、Write Through、Cache Aside。

**运行前环境要求**：

- Docker Desktop 已启动
- Redis 容器 `ch4-redis` 已运行并映射 `6379:6379`
- PostgreSQL 容器 `ch4-postgres` 已运行并映射 `5432:5432`
- PowerShell 中已设置 `PG_DSN`，或程序默认值与容器账号密码一致

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
