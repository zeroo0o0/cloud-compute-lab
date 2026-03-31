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
│  ├─ game_sticky_packets/
│  ├─ step3_framing_demo/
│  └─ TCP_reliable/
├─ exp4/
│  ├─ p2p_lockstep_host/
│  └─ p2p_lockstep_client/
├─ exp5/
│  ├─ cs_blocking_server/
│  ├─ cs_concurrent_server/
│  └─ cs_client/
├─ exp5_1/
│  ├─ zombie_server/
│  └─ zombie_client/
├─ exp6/
│  ├─ authoritative_server/
│  └─ authoritative_client/
└─ exp7/
   ├─ reliable_server/
   └─ reliable_client/
```

## 运行入口

- exp1: `go run ./cmd/exp1/loop`
- exp2:
  - `go run ./cmd/exp2/socket_server`
  - `go run ./cmd/exp2/socket_client`
- exp3:
  - `go run ./cmd/exp3/game_sticky_packets`
  - `go run ./cmd/exp3/step3_sticky_packets/server.go`
  - `go run ./cmd/exp3/step3_sticky_packets/client.go`
  - `go run ./cmd/exp3/step3_framing_demo/server.go`
  - `go run ./cmd/exp3/step3_framing_demo/client.go`
  - `go run ./cmd/exp3/TCP_reliable/server.go`
  - `go run ./cmd/exp3/TCP_reliable/player1.go`
  - `go run ./cmd/exp3/TCP_reliable/player2.go`
  - `TCP_reliable` 为三终端配合演示：服务器显示战场网格，玩家1 负责攻击，玩家2 负责断线与重连，用于展示“旧连接内传输可靠”与“新连接状态未恢复”是两件不同的事。
  
- exp4:
  - `go run ./cmd/exp4/p2p_lockstep_host`
  - `go run ./cmd/exp4/p2p_lockstep_client`
- exp5:
  - `go run ./cmd/exp5/cs_blocking_server`
  - `go run ./cmd/exp5/cs_concurrent_server`
  - `go run ./cmd/exp5/cs_client`
- exp5_1:
  - `go run ./cmd/exp5_1/zombie_server`
  - `go run ./cmd/exp5_1/zombie_client`
- exp6:
  - `go run ./cmd/exp6/authoritative_server`
  - `go run ./cmd/exp6/authoritative_client`
- exp7:
  - `go run ./cmd/exp7/reliable_server`
  - `go run ./cmd/exp7/reliable_client`

## 命名说明

- `exp5_1` 为 `exp5` 的扩展示例（僵尸连接/半开连接场景）。
- 当前目录保持原路径不变，避免影响既有讲义、脚本和运行命令。
