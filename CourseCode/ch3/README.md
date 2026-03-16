# 双雄对决演示实验 —— 代码与演示说明

> 本目录 (`ch3/`) 是一个**独立的 Go module**，包含 7 个阶段的可运行演示程序。
> 每个阶段均可在单机上直接运行；涉及网络的阶段(2/4/5/6/7)也支持多机运行。

---

## 目录结构

```
ch3/
├── go.mod                          # 独立模块 ch3
├── README.md                       # ← 本文件
├── 双雄对决演示实验.md              # 实验要求
├── internal/
│   ├── ch3proto/ch3proto.go        # 共享: 消息结构体 + sendJSON/recvJSON + JoinMsg
│   ├── ch3game/ch3game.go          # 共享: 确定性更新函数 DeterministicUpdate
│   ├── ch3net/ch3net.go            # 共享: ReliableConn (SetReadDeadline 封装)
│   └── ch3render/ch3render.go      # 共享: 地图渲染与状态格式化
└── cmd/
    ├── exp1/loop/                  # ① 单机游戏主循环
    ├── exp2/socket_server/         # ② TCP Socket 服务端
    ├── exp2/socket_client/         # ② TCP Socket 客户端
    ├── exp3/framing_demo/          # ③ 长度前缀+JSON 粘包解决
    ├── exp4/p2p_lockstep_host/     # ④ P2P 锁步 Host
    ├── exp4/p2p_lockstep_client/   # ④ P2P 锁步 Client
    ├── exp5/cs_blocking_server/    # ⑤ 阻塞服务器 (对照组)
    ├── exp5/cs_concurrent_server/  # ⑤ 并发服务器 (goroutine)
    ├── exp5/cs_client/             # ⑤ 通用客户端
    ├── exp6/authoritative_server/  # ⑥ 权威服务器
    ├── exp6/authoritative_client/  # ⑥ 权威客户端 (只发输入+渲染)
    ├── exp7/reliable_server/       # ⑦ ReliableConn 权威服务器
    └── exp7/reliable_client/       # ⑦ ReliableConn 客户端 (非阻塞收包)
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

# 可选：如果你的机器/环境里出现整型相关兼容问题，
# 可以先固定为 amd64 再编译（通常不是必须）
$env:GOARCH="amd64"

# 编译全部（验证无错误）
go build ./...
```

> `GOARCH=amd64` 是**可选配置**。大多数同学不需要设置；如果你在某些环境下遇到整型范围、位宽或编译兼容性问题，可以先加上这一行再运行。

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
cd ch3
go run ./cmd/exp1/loop
```

**操作**：输入 `w/a/s/d` 移动，`j` 攻击，`q` 退出。观察每次输入后坐标和状态刷新。

> 说明：Step1 已加入简易地图渲染，且坐标被限制在地图范围内（不允许负坐标）。

---

### Step 2 — 交互式 Socket 通信程序

**知识点**：`net.Listen` (绑定 `0.0.0.0` 通配 IP) / `listener.Accept()` / `net.Dial` / `conn.Write` / `conn.Read` / 长连接与终端交互 (`bufio.Scanner`)

#### 单机运行（两个终端）

```powershell
# 终端1 — 启动服务器
go run ./cmd/exp2/socket_server 

# 终端2 — 启动客户端
go run ./cmd/exp2/socket_client
# 提示输入 IP 时，直接按回车键（系统将默认连接本机 127.0.0.1:8888）
```
#### 多机运行（两个终端）
```powershell
# 机器A（服务器 / 教师端）
go run ./cmd/exp2/socket_server               # 默认监听 :8888，自动开放给整个局域网

# 机器B（客户端 / 学生端）
go run ./cmd/exp2/socket_client
# 启动后，根据控制台提示，手动输入机器A 的局域网 IP 地址 (例如 192.168.1.100)
```

---

### Step 3 — TCP 粘包与消息序列化（含正反面对比）

**知识点**：TCP 是无边界的连续字节流（水管模型）；利用4 字节大端序长度前缀 + `json.Marshal`”制定应用层通信契约，解决网络底层的粘包与半包问题。

**核心函数**：`sendJSON()`（`binary.Write` 写入 4 字节长度头） / `recvJSON()`（读取长度头 + `io.ReadFull` 精确截断字节流）

#### 演示粘包灾难版代码与正确处理版代码：

```powershell
# 运行粘包灾难版代码 
go run ./cmd/exp3/step3_sticky_packets

# 运行正确处理版代码
go run ./cmd/exp3/step3_framing_demo

```

---

### Step 4 — P2P 确定性帧同步双人网游

**知识点**：锁步循环（Lockstep）—— 发完输入必须阻塞等对方；`DeterministicUpdate()` 保证双方独立计算结果一致。

**核心函数**：`ch3game.DeterministicUpdate(state, local, remote, isHost)` — 根据 `isHost` 区分本地/远程输入，用 `dist <= 1` 判定命中扣血；任一方 `HP <= 0` 时结束游戏。

**显示方式**：除了输出 `(X,Y)` 坐标外，现在还会打印一个简易字符地图：

- `A`：玩家 P0
- `B`：玩家 P1
- `X`：两人重叠
- `.`：空地

#### 单机运行（两个终端）

```powershell
# 终端1 — Host
go run ./cmd/exp4/p2p_lockstep_host

# 终端2 — Client
go run ./cmd/exp4/p2p_lockstep_client
```

#### 多机运行

```powershell
# 机器A (Host)
go run ./cmd/exp4/p2p_lockstep_host            # 监听 :9104

# 机器B (Client)
go run ./cmd/exp4/p2p_lockstep_client 192.168.x.x
```

**操作**：双方轮流输入 `w/a/s/d/j`，观察：
1. 一方未输入时另一方阻塞（锁步）
2. 两边输出的 `state` 完全一致（确定性）
3. 当双方距离小于等于 1 时，按 `j` 可攻击，对方 HP 下降
4. 当任一方 `HP <= 0` 时，双方都会看到 `游戏结束`
5. 字符地图比单纯坐标更直观，适合讲解同步后的状态一致性

**规则说明总结**：

1. 双方距离小于等于 1 就可以进行攻击
2. 有一方 HP 小于等于 0 则游戏结束
3. 加入了地图展示，便于观察双方同步后的位置是否一致

---

### Step 5 — C/S 架构下的并发连接管理

**知识点**：单线程阻塞 vs `go handleClient()` 并发处理

**规则说明**：

1. 阻塞服务器一次只能处理一个客户端
2. 当已有客户端占用服务端时，后续客户端会收到“排队中/稍后重试”的通知
3. 并发服务器使用 `go handleClient(...)`，多个客户端互不阻塞

#### 演示对比

```powershell
# === 阻塞服务器（9105）===
# 终端1
go run ./cmd/exp5/cs_blocking_server

# 终端2/3/4 — 启动3个客户端
go run ./cmd/exp5/cs_client 127.0.0.1 9105

# 观察：只有第1个客户端能交互；后续客户端会收到“服务器忙/排队中”提示

# === 并发服务器（9106）===
# 终端1
go run ./cmd/exp5/cs_concurrent_server

# 终端2/3/4
go run ./cmd/exp5/cs_client 127.0.0.1 9106

# 观察：3个客户端同时交互，互不阻塞
```

#### 多机运行

服务器在一台机器运行，客户端改为 `go run ./cmd/exp5/cs_client <服务器IP> 9106`。

> 说明：
> - `9105` 是“阻塞服务器对照组”，设计目标就是让你观察“一个客户端占住服务端时，其它客户端只能排队”。

---

### Step 5.1 — TCP 半开连接（僵尸玩家）对游戏的影响（BP pressure）

**知识点**：TCP 半开连接（Half-Open）/ 僵尸连接：客户端“断网但不关进程”时，服务端可能既收不到数据也收不到断开通知；写入也可能在一段时间内“看似成功”（进入本机内核缓冲区），导致房间被僵尸玩家拖慢。

本实验仅保留 **BP（pressure 更强影响）** 模式：服务端在需要广播时，会对“疑似僵尸玩家（通常是 P1）”连接做大量发送以更容易填满发送缓冲区，从而让**正常玩家也感到延迟/卡顿**（讲解“对端不读导致 send 变慢/阻塞会拖累房间”）。

> 为了降低刷屏，本实验已调整为：**只有当玩家输入导致状态变化时，服务器才推进并广播**。

#### 单机运行（3个终端）

```powershell
# 终端1 — 服务器（:9110）
go run ./cmd/exp5_1/zombie_server

# 终端2 — 客户端 P0
go run ./cmd/exp5_1/zombie_client 127.0.0.1 0

# 终端3 — 客户端 P1
go run ./cmd/exp5_1/zombie_client 127.0.0.1 1
```

#### 演示操作

1. 两个客户端正常按键移动/攻击，确认每次按键后客户端才刷新（不再快速滚帧刷屏）。
2. 在其中一个客户端窗口按 **`t`**，切换到 `blackhole`（模拟断网/半开）：保持 TCP 连接不关闭，但**不收包也不发包**。
3. 观察正常玩家体感：
    - 正常玩家可能出现明显延迟/卡顿（因为服务器对僵尸连接的 send 被拖慢）。

#### 输出/终端说明（可选）

- 客户端默认会尝试用 ANSI 做“原地刷新”，避免刷屏；如果你拖动窗口大小时出现错位/残影，可在启动客户端前禁用 ANSI：

```powershell
$env:NO_ANSI=1
go run ./cmd/exp5_1/zombie_client 127.0.0.1 0
```

> 备注：本实验为了复现现象，刻意不加心跳/读超时；对照 Step7 的 `ReliableConn` 可进一步讲“如何用 timeout/重连避免僵尸玩家拖死房间”。

---

### Step 6 — 权威服务器游戏原型

**知识点**：服务器是唯一真相持有者——客户端只发输入、只渲染；`update()` + `broadcast()` 在服务端执行。

**输入方式**：客户端已改为 **raw 模式单键输入**，无需回车。

- `w/a/s/d`：移动
- `j`：攻击
- `q`：退出

#### 单机运行（3个终端）

```powershell
# 终端1 — 权威服务器
go run ./cmd/exp6/authoritative_server

# 终端2 — 客户端1
go run ./cmd/exp6/authoritative_client

# 终端3 — 客户端2
go run ./cmd/exp6/authoritative_client
```

#### 多机运行

```powershell
# 机器A
go run ./cmd/exp6/authoritative_server          # 监听 :9107

# 机器B/C
go run ./cmd/exp6/authoritative_client 192.168.x.x
```

**操作**：直接按 `w/a/s/d/j`，观察两个客户端都显示相同的权威状态。客户端代码中没有任何游戏逻辑计算，真正的状态更新全部发生在服务端。

**规则说明**：

1. 客户端只发送输入，不负责计算胜负和位置
2. 服务端维护唯一真相，两个客户端看到的状态必须一致
3. 输入方式为单键直发，无需回车，便于连续演示

---

### Step 7 — 健壮网络通信库 ReliableConn

**知识点**：`ReliableConn` 封装 `SetReadDeadline` 实现超时非阻塞收包；即使丢帧/延迟，主循环继续运行。

**输入方式**：客户端也已改为 **raw 模式单键输入**，无需回车。

- `w/a/s/d`：移动
- `j`：攻击
- `q`：主动退出客户端

**核心结构**（见 `internal/ch3net/ch3net.go`）：

```go
type ReliableConn struct { conn net.Conn }
func (rc *ReliableConn) Send(v any) error          // SendJSON 封装
func (rc *ReliableConn) Recv(timeout, out) error    // SetReadDeadline + RecvJSON
```

#### 单机运行（3个终端）

```powershell
# 终端1 — ReliableConn 服务器
go run ./cmd/exp7/reliable_server

# 终端2 — 客户端1
go run ./cmd/exp7/reliable_client 127.0.0.1 0

# 终端3 — 客户端2
go run ./cmd/exp7/reliable_client 127.0.0.1 1
```

#### 多机运行

```powershell
# 机器A
go run ./cmd/exp7/reliable_server                # 监听 :9108

# 机器B/C
go run ./cmd/exp7/reliable_client 192.168.x.x 0
```

**操作**：
1. 正常游戏：直接按 `w/a/s/d` 移动，按 `j` 攻击
2. **模拟掉线**：关闭一个客户端窗口，服务器不会停止，另一个客户端也不会把服务器拖死
3. 使用相同 `playerID`（如 `0` 或 `1`）重连后，玩家会恢复到掉线前的位置和血量
4. 控制台会渲染地图；掉线玩家会被标记为离线，重连后重新回到地图中
5. 与 Step4 对比，可以明显看出：这里使用了超时机制后，主循环不会因某个客户端异常而卡住

**规则说明**：

1. 客户端掉线不会直接结束服务器
2. 服务端会保留该玩家的状态（位置、血量）
3. 客户端必须使用固定 `playerID`（如 0/1）来重连并恢复之前状态，不能依赖重连顺序自动分配
4. 地图渲染会显示当前在线/离线玩家，逻辑更直观

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
| 4    | P2P 锁步 + 确定性计算        | `DeterministicUpdate`, `isHost`, 公共地图渲染 |
| 5    | 并发连接管理                 | `go handleClient(conn, id)`, 阻塞排队提示 |
| 6    | 权威服务器                   | 服务端 `update` + `broadcast`              |
| 7    | 超时非阻塞通信库             | `ReliableConn`, `SetReadDeadline`, 重连恢复状态 |
