# 裂土封疆演示实验（ch5）

`ch5/` 是第 5 章演示代码的独立 Go Module（`module ch5`），用于展示分布式系统中的关键机制与课堂实验流程。

---

## 目录结构（总览）

```text
ch5/
├── go.mod
├── README.md
├── 裂土封疆演示实验.md
├── cmd/
│   ├── exp6/
│   │   └── two_phase_commit/
│   │       ├── main.go
│   │       └── README.md
│   └── exp7/
│       └── raft_election/
│           └── main.go          ← 多线程 Raft，每个节点一个 goroutine
├── internal/
│   └── exp6_2pc/
│       ├── api.go
│       ├── core/
│       ├── scenario/
│       ├── utils/
│       └── demo_test.go
└── data/
    └── exp6_2pc/
```

---

## 内容说明

- `exp6`：分布式事务 Two-Phase Commit（2PC）
  - 支持正常流程与多类故障注入（拒票、超时、协调者崩溃、恢复重放）
  - 详细文档见：`cmd/exp6/two_phase_commit/README.md`

- `exp7`：Raft 领导者选举（Leader Election）
  - 每个节点在独立 goroutine 中运行，通过 channel 模拟 RequestVote / AppendEntries RPC
  - 3 节点随机超时选主，支持 Leader 宕机后自动故障转移演示
  - 核心算法逻辑集中在 main() 中，便于课堂讲解

- `裂土封疆演示实验.md`
  - 章节级实验要求与课堂观察目标（实验一到实验七）

---

## 启动步骤

在仓库根目录执行：

```powershell
cd .\CourseCode\ch5
```

### 实验六（2PC）

```powershell
go run ./cmd/exp6/two_phase_commit -scenario all -step-ms 900
go run ./cmd/exp6/two_phase_commit -scenario normal -step-ms 900
```

### 实验七（Raft 选主）

```powershell
go run ./cmd/exp7/raft_election -seed 7 -kill-after-ms 450
```

---

## 测试（可选）

```powershell
go test ./internal/exp6_2pc
```
