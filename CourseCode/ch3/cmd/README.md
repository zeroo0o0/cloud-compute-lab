# ch3/cmd 目录索引（不改程序版）

本目录仅做结构整理说明，不改动任何程序代码与运行路径。

## 目录总览

```text
cmd/
├─ exp1/
│  └─ loop/
├─ exp2/
│  ├─ socket_server/
│  └─ socket_client/
├─ exp3/
│  ├─ step3_sticky_packets/
│  │  ├─ server/
│  │  └─ client/
│  ├─ game_sticky_packets/
│  │  ├─ server/
│  │  └─ client/
│  ├─ step3_framing_demo/
│  │  ├─ server/
│  │  └─ client/
│  └─ TCP_reliable/
│     ├─ server/
│     ├─ player1/
│     └─ player2/
├─ exp4/
│  ├─ p2p_lockstep_host/
│  └─ p2p_lockstep_client/
├─ exp5/
│  ├─ cs_blocking_server/
│  ├─ cs_concurrent_server/
│  └─ cs_client/
├─ exp5_1/
│  ├─ single_thread_server/
│  │  ├─ zombie_server/
│  │  └─ zombie_client/
│  └─ multi_thread_server/
│     ├─ zombie_server/
│     └─ zombie_client/
├─ exp6/
│  ├─ authoritative_server/
│  └─ authoritative_client/
└─ exp7/
  ├─ single_thread/
  │  ├─ reliable_server/
  │  └─ reliable_client/
  └─ multi_thread/
    ├─ reliable_server/
    └─ reliable_client/
```

## 运行入口

- exp1：实验一：本地游戏循环
  - `go run ./cmd/exp1/loop`
- exp2：实验二：TCP Socket 长连接通信
  - `go run ./cmd/exp2/socket_server`
  - `go run ./cmd/exp2/socket_client`
- exp3：实验三：TCP 字节流边界与会话状态
  - `go run ./cmd/exp3/game_sticky_packets/server`
  - `go run ./cmd/exp3/game_sticky_packets/client`
  - `go run ./cmd/exp3/step3_sticky_packets/server`
  - `go run ./cmd/exp3/step3_sticky_packets/client`
  - `go run ./cmd/exp3/step3_framing_demo/server`
  - `go run ./cmd/exp3/step3_framing_demo/client`
  - `go run ./cmd/exp3/TCP_reliable/server`
  - `go run ./cmd/exp3/TCP_reliable/player1`
  - `go run ./cmd/exp3/TCP_reliable/player2`
  - `TCP_reliable` 为三终端配合演示：服务器显示战场网格，玩家1 负责攻击，玩家2 负责断线与重连，用于展示“旧连接内传输可靠”与“新连接状态未恢复”是两件不同的事。
  
- exp4：实验四：P2P 确定性锁步
  - `go run ./cmd/exp4/p2p_lockstep_host`
  - `go run ./cmd/exp4/p2p_lockstep_client`
- exp5：实验五：C/S 并发连接管理
  - `go run ./cmd/exp5/cs_blocking_server`
  - `go run ./cmd/exp5/cs_concurrent_server`
  - `go run ./cmd/exp5/cs_client`
- exp5_1：扩展实验：半开连接与僵尸玩家
  - `go run ./cmd/exp5_1/single_thread_server/zombie_server`
  - `go run ./cmd/exp5_1/single_thread_server/zombie_client`
  - `go run ./cmd/exp5_1/multi_thread_server/zombie_server`
  - `go run ./cmd/exp5_1/multi_thread_server/zombie_client`
- exp6：实验六：权威服务器状态同步
  - `go run ./cmd/exp6/authoritative_server`
  - `go run ./cmd/exp6/authoritative_client`
- exp7：实验七：超时通信与断线恢复
  - `go run ./cmd/exp7/single_thread/reliable_server`
  - `go run ./cmd/exp7/single_thread/reliable_client`
  - `go run ./cmd/exp7/multi_thread/reliable_server`
  - `go run ./cmd/exp7/multi_thread/reliable_client`

## 命名说明

- `exp1` 到 `exp7` 对应 ch3 主 README 中的实验一到实验七。
- `exp5_1` 为实验五的扩展示例（半开连接/僵尸玩家场景）。
- `single_thread_server` 演示单线程 ticker 轮询读输入与广播时的读阻塞影响。
- `multi_thread_server` 保留原有 goroutine 收包版本（`zombie_client`/`zombie_server`）。

## exp5_1 单线程版说明

- `single_thread_server/zombie_server` 端口为 `:9107`，按顺序接入 2 个客户端并在主循环中阻塞读输入。
- `single_thread_server/zombie_client` 支持输入 `t` 模拟断网（不收不发），用于观察服务端阻塞现象。
