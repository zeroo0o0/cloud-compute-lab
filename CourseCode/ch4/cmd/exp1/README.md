# 实验一：突破单线程瓶颈

本实验现在按 1-6 的顺序组织：奇数步骤先看最小原理，偶数步骤再嵌入 Warzone 极简游戏场景。网络版场景压缩为 Warzone 的“玩家 ACTION -> 权威 PlayerState -> STATE_UPDATE”流程。

---

## 目录结构

```text
exp1/
├─ 01_basic_serial_blocking_demo/       # 1. 最小原理：串行读慢输入会拖住快输入
├─ 02_network_serial_warzone/           # 2. Warzone 嵌入：串行收包反例
│  ├─ server/
│  └─ client/
├─ 03_basic_goroutine_receiver_demo/    # 3. 最小原理：每个输入源用独立 goroutine 读取
├─ 04_network_goroutine_warzone/        # 4. Warzone 嵌入：goroutine 解耦收包
│  ├─ server/
│  └─ client/
├─ 05_basic_event_driven_demo/          # 5. 最小原理：有事件就处理，没事件就继续 tick
└─ 06_network_event_driven_warzone/     # 6. Warzone 嵌入：事件驱动 + 增量同步
   ├─ server/
   └─ client/
```

---

## 步骤 1：串行阻塞最小原理

```powershell
go run ./cmd/exp1/01_basic_serial_blocking_demo
```

知识点对应代码：

- `01_basic_serial_blocking_demo/main.go`：`readSlowInput()` 先执行，`readFastInput()` 后执行。
- 这个顺序会导致 fast 本身只需要 20ms，也要排在 slow 的 500ms 读取之后。
- 这段只讲“串行等待导致排队”，没有任何游戏、网络协议和多玩家状态。

## 步骤 2：串行阻塞嵌入 Warzone

```powershell
# 终端1：服务端
go run ./cmd/exp1/02_network_serial_warzone/server

# 终端2：快玩家
go run ./cmd/exp1/02_network_serial_warzone/client -player fast

# 终端3：慢玩家
go run ./cmd/exp1/02_network_serial_warzone/client -player slow
```

知识点对应代码：

- `02_network_serial_warzone/server/main.go`：`runFrameSerial` 函数先调用 `readAction(sessions["slow"].reader)`，再调用 `readAction(sessions["fast"].reader)`。
- `02_network_serial_warzone/server/main.go`：`gameState.applyAction` 模拟 Warzone 服务端修改权威 `PlayerState`，`broadcastState` 模拟 `STATE_UPDATE` 广播。
- `02_network_serial_warzone/client/main.go`：客户端发送 `ACTION fast MOVE_RIGHT ...` 或 `ACTION slow MOVE_LEFT ...`，并用 `defaultDelay` 制造 `fast=20ms`、`slow=500ms`。
- 解决/暴露的游戏问题：如果服务器主循环串行读每个玩家连接，断流骑士 slow 会让疾风游侠 fast 的 ACTION 排队到 500ms 后才生效。

---

## 步骤 3：goroutine 解耦最小原理

```powershell
go run ./cmd/exp1/03_basic_goroutine_receiver_demo
```

知识点对应代码：

- `03_basic_goroutine_receiver_demo/main.go`：`go receive("slow", slowInput, results)` 和 `go receive("fast", fastInput, results)`。
- 主循环启动两个 goroutine 后立刻继续执行；慢输入只阻塞自己的 goroutine。
- 这段只讲“把等待从主循环拆出去”，不讲游戏状态同步。

## 步骤 4：goroutine 解耦嵌入 Warzone

```powershell
# 终端1：服务端
go run ./cmd/exp1/04_network_goroutine_warzone/server

# 终端2：快玩家
go run ./cmd/exp1/04_network_goroutine_warzone/client -player fast

# 终端3：慢玩家
go run ./cmd/exp1/04_network_goroutine_warzone/client -player slow
```

知识点对应代码：

- `04_network_goroutine_warzone/server/main.go`：`runFrameWithGoroutines` 函数中，`for _, s := range sessions { go func(...) { ... }(s) }` 为每条玩家连接启动收包 goroutine。
- `04_network_goroutine_warzone/server/main.go`：`results := make(chan actionResult, len(sessions))` 是收包 goroutine 把 ACTION 交回主循环的通道。
- 解决的游戏问题：断流骑士 slow 仍然会晚到，但只阻塞自己的连接 goroutine；疾风游侠 fast 的 ACTION 可以先被应用到权威世界状态。

---

## 步骤 5：事件驱动最小原理

```powershell
go run ./cmd/exp1/05_basic_event_driven_demo
```

知识点对应代码：

- `05_basic_event_driven_demo/main.go`：每个 tick 中的 `select { case ev := <-events: ... default: ... }`。
- 有事件时处理事件；没有事件时进入 `default`，打印“没有事件”，然后直接进入下一 tick。
- 这段专门让学生看“有事件”和“没事件”的区别，不混入游戏网络细节。

## 步骤 6：事件驱动与增量同步嵌入 Warzone

```powershell
# 终端1：服务端
go run ./cmd/exp1/06_network_event_driven_warzone/server

# 终端2：快玩家
go run ./cmd/exp1/06_network_event_driven_warzone/client -player fast

# 终端3：慢玩家
go run ./cmd/exp1/06_network_event_driven_warzone/client -player slow
```

知识点对应代码：

- `06_network_event_driven_warzone/server/main.go`：`runEventDrivenLoop` 函数按 `ticker` 推进 tick。
- `06_network_event_driven_warzone/server/main.go`：tick 内部的 `select/default` 只取当前已经到达的 ACTION；没有 ACTION 时不等待。
- `06_network_event_driven_warzone/server/main.go`：`dirtyPlayers` 只记录本 tick 状态发生变化的玩家，演示增量 `STATE_UPDATE`。
- 解决的游戏问题：服务器不用为了等断流骑士 slow 暂停整个世界；疾风游侠 fast 的移动可以先同步出去；没有变化的 tick 不做无意义全量广播。

---

## 课堂提醒

- 本实验故意只保留 `fast` 和 `slow` 两个玩家，因为两个输入源已经足够说明“谁在阻塞谁”。
- 网络嵌入版的 server 和 client 已拆成同一步骤目录下的 `server/main.go` 与 `client/main.go`。
- 如果要调整慢玩家延迟，可以在客户端加参数，例如 `-delay-ms 800`。
