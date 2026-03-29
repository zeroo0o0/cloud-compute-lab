# 双雄对决演示实验 —— 代码与演示说明

> 本目录 (`ch3/`) 是一个**独立的 Go module**，包含 7 个阶段的可运行演示程序。
> 每个阶段均可在单机上直接运行；涉及网络的阶段(2/4/5/6/7)也支持多机运行。

---

## 目录结构

```
ch3/
├── go.mod                              # 独立模块 ch3
├── README.md                           # ← 本文件
├── 双雄对决演示实验.md                  # 实验要求
├── internal/
│   ├── ch3proto/ch3proto.go            # 共享: 消息结构体 + sendJSON/recvJSON + JoinMsg
│   ├── ch3game/ch3game.go              # 共享: 确定性更新函数 DeterministicUpdate
│   ├── ch3net/ch3net.go                # 共享: ReliableConn (SetReadDeadline 封装)
│   └── ch3render/ch3render.go          # 共享: 地图渲染与状态格式化
└── cmd/
    ├── README.md                       # cmd 子目录说明
    ├── exp1/loop/                      # ① 单机游戏主循环
    ├── exp2/socket_server/             # ② TCP Socket 服务端
    ├── exp2/socket_client/             # ② TCP Socket 客户端
    ├── exp3/game_sticky_packets/       # ③ 粘包灾难版（游戏演示）
    ├── exp3/step3_sticky_packets/      # ③ 粘包灾难版（纯文本）
    ├── exp3/step3_framing_demo/        # ③ 长度前缀+JSON 正确处理
    ├── exp3/TCP_reliable/              # ③ TCP 可靠性演示（server/player1/player2）
    ├── exp4/p2p_lockstep_host/         # ④ P2P 锁步 Host
    ├── exp4/p2p_lockstep_client/       # ④ P2P 锁步 Client
    ├── exp5/cs_blocking_server/        # ⑤ 阻塞服务器（对照组）
    ├── exp5/cs_concurrent_server/      # ⑤ 并发服务器（goroutine）
    ├── exp5/cs_client/                 # ⑤ 通用客户端
    ├── exp5_1/zombie_server/           # ⑤.1 僵尸连接（半开连接）服务端
    ├── exp5_1/zombie_client/           # ⑤.1 僵尸连接（半开连接）客户端
    ├── exp6/authoritative_server/      # ⑥ 权威服务器
    ├── exp6/authoritative_client/      # ⑥ 权威客户端（只发输入+渲染）
    ├── exp7/reliable_server/           # ⑦ ReliableConn 权威服务器
    └── exp7/reliable_client/           # ⑦ ReliableConn 客户端（非阻塞收包）
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

**知识点**：TCP 是无边界的连续字节流；利用4 字节大端序长度前缀 + `json.Marshal`”制定应用层通信契约，解决网络底层的粘包与半包问题。
除“TCP 字节流无边界导致粘包/半包”外，本步还通过 `TCP_reliable` 演示：`conn.Write` 返回成功不等于对端已真实处理；客户端断线重连后若无会话恢复与状态同步，容易出现“服务器判死、客户端满血重连”的状态冲突。

**核心函数**：`sendJSON()` / `recvJSON()`（长度前缀拆包）；`net.Listen` + `listener.Accept`（连接管理）；`conn.Read` / `conn.Write`（演示断线广播与重连后的状态不一致问题）

#### 演示顺序（建议按 1 → 2 → 3）
#### 演示 1：游戏化粘包现象（自动单机）

```powershell
go run ./cmd/exp3/game_sticky_packets
```

**现象**：第一条移动可被解析，后续两条在底层合并后形成连体 JSON，触发解析错误，角色停在中间帧。

#### 演示 2：纯文本粘包灾难版（两个终端）

```powershell
# 终端1：服务端（故意用 conn.Read 直接读裸流，不做消息边界）
go run ./cmd/exp3/step3_sticky_packets/server.go

# 终端2：客户端（连续发送多条 JSON，未加长度头）
go run ./cmd/exp3/step3_sticky_packets/client.go
```

在mac系统中，可以通过如下命令来关闭回环网卡从而模拟网络故障：
```shell
sudo ifconfig lo0 down
```

通过下面的命令恢复：
```shell
sudo ifconfig lo0 up
```

检验回环网卡是否关闭：
```shell
ping 127.0.0.1
```

#### 演示 3：长度前缀 + JSON 正确版（两个终端）

```powershell
# 终端1：服务端（recvJSON 按长度拆包）
go run ./cmd/exp3/step3_framing_demo/server.go

# 终端2：客户端（sendJSON 先写长度头再写 payload）
go run ./cmd/exp3/step3_framing_demo/client.go
```

**现象**：即使连续发送消息，服务端仍能按条稳定解包并逐条打印。

#### 演示 4：TCP_reliable（三个终端，重连状态冲突演示）

```powershell
# 终端1：服务器
go run ./cmd/exp3/TCP_reliable/server.go

# 终端2：玩家1（正常）
go run ./cmd/exp3/TCP_reliable/player1.go

# 终端3：玩家2（先断线再重连）
go run ./cmd/exp3/TCP_reliable/player2.go
```

**观察点**：
1. 玩家1会收到“Player2 DEAD”状态广播。
2. 玩家2断线后重连，服务端会检测到“新连接”，并打印状态冲突提示（用于讲解仅靠 TCP 连接可靠性不足以保证游戏状态一致性）。

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
# 终端1（先启动）
go run ./cmd/exp5/cs_blocking_server

# 终端2（先连上并持续交互，不要立刻退出）
go run ./cmd/exp5/cs_client 127.0.0.1 9105

# 终端3/4（随后再启动）
go run ./cmd/exp5/cs_client 127.0.0.1 9105

# 观察：只有终端2能持续交互；终端3/4会收到“服务器忙/排队中”提示

# === 并发服务器（9106）===
# 终端1（重开一个新服务）
go run ./cmd/exp5/cs_concurrent_server

# 终端2/3/4 再次分别连接
go run ./cmd/exp5/cs_client 127.0.0.1 9106

# 观察：3个客户端同时交互，互不阻塞
```

#### 课堂演示口径

1. 先讲阻塞版：服务端一次只处理一个连接，后来的连接只能排队。
2. 让终端2持续输入，确认它“独占”了服务端处理能力。
3. 启动终端3/4，强调它们连接到了端口，但业务处理被前一个客户端阻塞。
4. 切换到并发版后，重复同样操作，展示三个客户端都可同时交互。

#### 预期现象

1. 阻塞版（9105）：
    - 第一个客户端响应正常。
    - 后续客户端提示忙碌/排队，体感明显延迟或无法进入交互。
2. 并发版（9106）：
    - 多客户端可同时收发。
    - 一个客户端慢操作不会阻塞其他客户端。

#### 多机运行

服务器在一台机器运行，客户端改为 `go run ./cmd/exp5/cs_client <服务器IP> 9106`。

> 说明：
> - `9105` 是“阻塞服务器对照组”，设计目标就是让你观察“一个客户端占住服务端时，其它客户端只能排队”。

---

### Step 5.1 — TCP 半开连接（僵尸玩家）对游戏的影响（两种场景）

**知识点**：TCP 半开连接（Half-Open）/ 僵尸连接：客户端“断网但不关进程”时，服务端可能既收不到数据也收不到断开通知；写入也可能在一段时间内“看似成功”（仅进入本机内核缓冲区），导致服务器误判在线。

本实验现在提供两个可切换模式，分别对应课堂上常见的两类故障：

1. `read-block`：服务器最大连接数固定为 2。每个连接的接收协程都在 `RecvJSON/read` 阻塞等待输入；僵尸玩家断网但不发送 FIN 时，会长期占住连接槽位，第三个玩家无法进入，但另一个活跃玩家仍可继续游戏。
2. `write-illusion`：服务器没有心跳机制，且会持续向僵尸连接写数据。写调用在一段时间内可能“看起来成功”（仅进入内核发送缓冲区），当缓冲区逐步占满后，正常玩家也会被拖慢甚至卡死。

#### 单机运行（4个终端，建议）

```powershell
# 终端1 — 服务器（:9110）
# 场景A：阻塞读卡死
go run ./cmd/exp5_1/zombie_server read-block

# 或 场景B：写入假在线
go run ./cmd/exp5_1/zombie_server write-illusion

# 终端2 — 客户端 P0
go run ./cmd/exp5_1/zombie_client 127.0.0.1 0

# 终端3 — 客户端 P1
go run ./cmd/exp5_1/zombie_client 127.0.0.1 1

# 终端4 — 第三个玩家（用于观察“无法进入房间”）
go run ./cmd/exp5_1/zombie_client 127.0.0.1 0
```

#### 演示操作

1. 两个客户端先正常操作，确认可同步移动/攻击。
2. 让 P1 客户端按 **`t`** 切换 `blackhole`（模拟断网/半开）：保持 TCP 连接不关闭，但不收包也不发包。
3. 观察 `read-block` 模式：
    - 僵尸玩家对应连接会长期卡在 `RecvJSON/read` 等待，CPU 占用仍较低（阻塞等待）。
    - 客户端2（正常玩家）仍可继续移动/攻击，不会被客户端1的断网直接拖死。
    - 如果客户端1是“半开僵尸”（不发不收且不关闭连接），此时启动第 3 个客户端会收到 `ROOM FULL`（槽位被僵尸占住）。
    - 如果客户端1是“正常退出”（按 `q` 或进程结束并关闭连接），该槽位会被释放，第 3 个客户端可成功加入并复用该槽位。
4. 观察 `write-illusion` 模式：
    - 客户端状态栏 `event` 会持续显示类似 `WRITE-ILLUSION ... p1 silence=xx.xs`。
    - 地图中 P1 仍显示 `online`（无心跳、无断线确认）。
    - 服务器会在 P1 长时间沉默后，对僵尸连接高压发送，并将其发送缓冲区调小（示例 4KB）；随着缓冲区被占满，正常玩家更新会明显变慢甚至卡住。
    - 输入按键按一次只生效一次（不会出现“按一次 d 一直向右移动”的粘滞输入）。

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

本步骤重点验证三件事：
1. 使用长度前缀 + JSON，避免 TCP 粘包/半包导致的反序列化错位。
2. 客户端出现丢帧/延迟时不会“假死”或状态崩坏；服务器端某帧收不到输入也不会阻塞主循环。
3. 客户端与服务器都采用超时读取，超时时本帧按“无输入/无新状态”处理，循环继续。

**输入方式**：客户端也已改为 **raw 模式单键输入**，无需回车。

- `w/a/s/d`：移动
- `j`：攻击
- `q`：主动退出客户端
- `t`：切换“本地丢帧模拟”（客户端主动跳过部分收包）
- `p`：切换“防粘包开关”（ON=长度前缀正确解析，OFF=模拟无边界解析）
- `u`：触发“突发发送测试”（客户端一次性快速发送多条输入消息）

**核心结构**（见 `internal/ch3net/ch3net.go`）：

```go
type ReliableConn struct {
    conn    net.Conn
    writeMu sync.Mutex
}
func (rc *ReliableConn) Send(v any) error
func (rc *ReliableConn) SendTimeout(timeout time.Duration, v any) error
func (rc *ReliableConn) Recv(timeout, out) error
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
5. 在任一客户端按 `u` 可触发突发发送，观察高频消息流下依旧正常解析
6. 与 Step4 对比，可以明显看出：这里使用了超时机制后，主循环不会因某个客户端异常而卡住

#### 三种鲁棒性演示（课堂建议按此顺序）

1. 粘包/序列化正确性
    - 操作：正常启动 server + 两个 client；在某个 client 先按 `p` 关闭防粘包，再按 `u` 触发突发发送。
    - 观察（`p=OFF`）：界面会出现明显的状态卡顿/更新丢失，模拟无边界解析下的粘包灾难。
    - 再操作：再次按 `p` 打开防粘包，再按 `u`。
    - 观察（`p=ON`）：角色会连续位移/攻击，界面稳定。
    - 原因：通信统一走长度前缀 + JSON（`SendJSON/RecvJSON`），即使底层出现粘包/半包也能正确拆包。

2. 丢帧/延迟不致命（客户端与服务器）
    - 操作A（客户端侧）：在某个客户端按 `t` 开启本地丢帧模拟。
    - 观察A：该客户端画面更新会变稀疏，但不会掉线；关闭 `t` 后可继续同步。
    - 操作B（服务器侧）：让某个客户端短时间不操作，或临时关闭一个客户端。
    - 观察B：服务器 frame 日志继续增长；另一个在线客户端仍可正常交互。
    - 结论：单帧收不到输入只会按“该帧无输入”处理，不会阻塞主循环。

3. 读取超时与非阻塞
    - 操作：观察 server/client 代码中的 `Recv(timeout, ...)` 路径（server 500ms，client 50ms），并在运行时制造短暂无输入窗口。
    - 观察：超时分支只会跳过本次读取，循环继续；不会出现“卡死等包”。
    - 结论：超时机制将“网络等待”从阻塞问题降级为“本帧无新数据”。

**规则说明**：

1. 客户端掉线不会直接结束服务器
2. 服务端会保留该玩家的状态（位置、血量）
3. 客户端必须使用固定 `playerID`（如 0/1）来重连并恢复之前状态，不能依赖重连顺序自动分配
4. 地图渲染会显示当前在线/离线玩家，逻辑更直观
5. 服务器广播对单个慢连接采用发送超时；慢连接最多丢帧，不会长期拖慢整个主循环

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
