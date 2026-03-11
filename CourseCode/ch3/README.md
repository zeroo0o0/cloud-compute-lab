# 双雄对决演示实验 —— 代码与演示说明

> 本目录 (`ch3/`) 是一个**独立的 Go module**，包含 7 个阶段的可运行演示程序。
> 每个阶段均可在单机上直接运行；涉及网络的阶段(2/4/5/6/7)也支持多机运行。

---

## 目录结构

```
ch3/
├── go.mod                          # 独立模块 warzone/exp6
├── README.md                       # ← 本文件
├── 双雄对决演示实验.md              # 实验要求
├── internal/
│   ├── exp6proto/messages.go       # 共享: 消息结构体 + sendJSON/recvJSON
│   ├── exp6game/deterministic.go   # 共享: 确定性更新函数 DeterministicUpdate
│   └── exp6net/reliable_conn.go    # 共享: ReliableConn (SetReadDeadline 封装)
└── cmd/
    ├── step1_loop/                 # ① 单机游戏主循环
    ├── step2_socket_server/        # ② TCP Socket 服务端
    ├── step2_socket_client/        # ② TCP Socket 客户端
    ├── step3_framing_demo/         # ③ 长度前缀+JSON 粘包解决
    ├── step4_p2p_lockstep_host/    # ④ P2P 锁步 Host
    ├── step4_p2p_lockstep_client/  # ④ P2P 锁步 Client
    ├── step5_cs_blocking_server/   # ⑤ 阻塞服务器 (对照组)
    ├── step5_cs_concurrent_server/ # ⑤ 并发服务器 (goroutine)
    ├── step5_cs_client/            # ⑤ 通用客户端
    ├── step6_authoritative_server/ # ⑥ 权威服务器
    ├── step6_authoritative_client/ # ⑥ 权威客户端 (只发输入+渲染)
    ├── step7_reliable_server/      # ⑦ ReliableConn 权威服务器
    └── step7_reliable_client/      # ⑦ ReliableConn 客户端 (非阻塞收包)
```

---

## 环境搭建

### 前提条件

| 依赖 | 版本    | 说明                   |
|------|---------|----------------------|
| Go   | ≥ 1.21 | 标准库即可，无第三方依赖 |

### 首次构建

```powershell
# 进入 ch3 目录
cd ch3

# 编译全部（验证无错误）
go build ./...
```

如果编译失败遇到了类似这样的错误

```shell
# warzone/exp6/internal/exp6proto
internal\exp6proto\messages.go:43:18: constant 4294967295 overflows int
```

运行下面的指令：
```shell
$env:GOARCH="amd64" 
```

### 多机运行网络配置

- 查看服务器 IP：`ipconfig`（Windows）或 `ip addr show`（Linux）
- 防火墙放行：`netsh advfirewall firewall add rule name="exp6" dir=in action=allow protocol=tcp localport=9102-9108`
- 客户端连接时将 `127.0.0.1` 替换为服务器 IP 即可

---

## 各步骤演示操作

### Step 1 — 单机本地游戏原型

**知识点**：Game Loop（输入 → 计算 → 渲染）

**核心函数**：`readInput()` → `updateState()` → `render()`

```powershell
cd exp6
go run ./cmd/step1_loop
```

**操作**：输入 `w/a/s/d` 移动，`j` 攻击，`q` 退出。观察每次输入后坐标和状态刷新。

---

### Step 2 — Socket 通信程序

**知识点**：`net.Listen` / `listener.Accept()` / `net.Dial` / `conn.Write` / `conn.Read`

#### 单机运行（两个终端）

```powershell
# 终端1 — 启动服务器
go run ./cmd/step2_socket_server

# 终端2 — 启动客户端
go run ./cmd/step2_socket_client
```

#### 多机运行

```powershell
# 机器A（服务器）
go run ./cmd/step2_socket_server              # 监听 :9102

# 机器B（客户端）— 将 IP 替换为机器A 的地址
go run ./cmd/step2_socket_client 192.168.x.x
```

**操作**：客户端输入文字，观察服务端回显。

---

### Step 3 — TCP 粘包与消息序列化

**知识点**：TCP 是无边界字节流；用 4 字节长度前缀 + `json.Marshal` 传结构化消息。

**核心函数**：`exp6proto.SendJSON()` / `exp6proto.RecvJSON()`（见 `internal/exp6proto/messages.go`）

```powershell
go run ./cmd/step3_framing_demo
```

**观察输出**：客户端连续发送 3 条 JSON 消息，服务端逐条正确解析并回复——证明长度前缀解决了粘包。

---

### Step 4 — P2P 确定性帧同步双人网游

**知识点**：锁步循环（Lockstep）—— 发完输入必须阻塞等对方；`DeterministicUpdate()` 保证双方独立计算结果一致。

**核心函数**：`exp6game.DeterministicUpdate(state, local, remote, isHost)` — 根据 `isHost` 区分本地/远程输入，用 `dist < 1` 判定命中扣血。

#### 单机运行（两个终端）

```powershell
# 终端1 — Host
go run ./cmd/step4_p2p_lockstep_host

# 终端2 — Client
go run ./cmd/step4_p2p_lockstep_client
```

#### 多机运行

```powershell
# 机器A (Host)
go run ./cmd/step4_p2p_lockstep_host           # 监听 :9104

# 机器B (Client)
go run ./cmd/step4_p2p_lockstep_client 192.168.x.x
```

**操作**：双方轮流输入 `w/a/s/d/j`，观察：
1. 一方未输入时另一方阻塞（锁步）
2. 两边输出的 `state` 完全一致（确定性）
3. 靠近后按 `j` 攻击，对方 HP 下降

---

### Step 5 — C/S 架构下的并发连接管理

**知识点**：单线程阻塞 vs `go handleClient()` 并发处理

#### 演示对比

```powershell
# === 阻塞服务器（9105）===
# 终端1
go run ./cmd/step5_cs_blocking_server

# 终端2/3/4 — 启动3个客户端
go run ./cmd/step5_cs_client 127.0.0.1 9105

# 观察：只有第1个客户端能交互，后续客户端卡住

# === 并发服务器（9106）===
# 终端1
go run ./cmd/step5_cs_concurrent_server

# 终端2/3/4
go run ./cmd/step5_cs_client 127.0.0.1 9106

# 观察：3个客户端同时交互，互不阻塞
```

#### 多机运行

服务器在一台机器运行，客户端改为 `go run ./cmd/step5_cs_client <服务器IP> 9106`。

---

### Step 6 — 权威服务器游戏原型

**知识点**：服务器是唯一真相持有者——客户端只发输入、只渲染；`update()` + `broadcast()` 在服务端执行。

#### 单机运行（3个终端）

```powershell
# 终端1 — 权威服务器
go run ./cmd/step6_authoritative_server

# 终端2 — 客户端1
go run ./cmd/step6_authoritative_client

# 终端3 — 客户端2
go run ./cmd/step6_authoritative_client
```

#### 多机运行

```powershell
# 机器A
go run ./cmd/step6_authoritative_server         # 监听 :9107

# 机器B/C
go run ./cmd/step6_authoritative_client 192.168.x.x
```

**操作**：输入 `w/a/s/d`，观察两个客户端都显示相同的权威状态。客户端代码中没有任何游戏逻辑计算。

---

### Step 7 — 健壮网络通信库 ReliableConn

**知识点**：`ReliableConn` 封装 `SetReadDeadline` 实现超时非阻塞收包；即使丢帧/延迟，主循环继续运行。

**核心结构**（见 `internal/exp6net/reliable_conn.go`）：

```go
type ReliableConn struct { conn net.Conn }
func (rc *ReliableConn) Send(v any) error          // SendJSON 封装
func (rc *ReliableConn) Recv(timeout, out) error    // SetReadDeadline + RecvJSON
```

#### 单机运行（3个终端）

```powershell
# 终端1 — ReliableConn 服务器
go run ./cmd/step7_reliable_server

# 终端2 — 客户端1
go run ./cmd/step7_reliable_client

# 终端3 — 客户端2
go run ./cmd/step7_reliable_client
```

#### 多机运行

```powershell
# 机器A
go run ./cmd/step7_reliable_server               # 监听 :9108

# 机器B/C
go run ./cmd/step7_reliable_client 192.168.x.x
```

**操作**：
1. 正常游戏：`w/a/s/d` 移动，`j` 攻击（靠近后扣血）
2. **模拟掉线**：暂停/关闭一个客户端窗口，观察服务器和另一个客户端不会卡死（对比 Step4 的锁步阻塞）
3. 观察客户端输出偶尔的 "超时丢帧" 但循环照常运行

---

## 各步骤端口汇总

| 步骤 | 端口 | 说明               |
|------|------|--------------------|
| 2    | 9102 | Socket 通信         |
| 3    | 9103 | 粘包演示 (自动单机)  |
| 4    | 9104 | P2P 锁步           |
| 5    | 9105 | 阻塞服务器          |
| 5    | 9106 | 并发服务器          |
| 6    | 9107 | 权威服务器          |
| 7    | 9108 | ReliableConn 服务器 |

> 多机运行时确保对应端口在防火墙中已放行。

---

## 核心知识点对照表

| 步骤 | 演示目标                     | 关键函数/结构体                             |
|------|-----------------------------|--------------------------------------------|
| 1    | Game Loop                   | `readInput → updateState → render`         |
| 2    | TCP Socket 建连与收发        | `net.Listen`, `net.Dial`, `conn.Read/Write`|
| 3    | 粘包解决 + 序列化            | `sendJSON`, `recvJSON`, `binary.Write`     |
| 4    | P2P 锁步 + 确定性计算        | `DeterministicUpdate`, `isHost`            |
| 5    | 并发连接管理                 | `go handleClient(conn, id)`               |
| 6    | 权威服务器                   | 服务端 `update` + `broadcast`              |
| 7    | 超时非阻塞通信库             | `ReliableConn`, `SetReadDeadline`          |
