# 裂土封疆演示实验（ch5）

`ch5/` 是第 5 章演示代码的独立 Go Module（`module ch5`），用于展示分布式系统中的关键机制与课堂实验流程。

---

## 目录结构（总览）

```text
ch5/
├── go.mod
├── README.md
├── 裂土封疆演示实验.md
├── exp6/                     ← 2PC 可视化简化版（逐步演示）
│   ├── main.go
│   └── README.md
├── exp6_bonus/               ← 2PC 进阶版（完整日志与剧情）
│   ├── main.go
│   ├── README.md
│   ├── exp6_2pc/
│   └── data/
└── exp7/                     ← 多线程 Raft（可视化逐步演示）
  ├── main.go
  └── README.md
```

---

## 内容说明

- `exp6`：2PC 可视化简化版（逐步演示）
  - 结构化展示 1 协调者 + 3 参与者状态
  - 每步按 Enter 推进，便于课堂讲解

- `exp6_bonus`：2PC 进阶版（完整日志与剧情渲染）
  - 支持正常流程与多类故障注入（拒票、超时、协调者崩溃、恢复重放）
  - 详细文档见：`exp6_bonus/README.md`

- `exp7`：Raft 领导者选举（Leader Election）
  - 每个节点在独立 goroutine 中运行，通过 channel 模拟 RequestVote / AppendEntries RPC
  - 3 节点随机超时选主，支持 Leader 宕机后自动故障转移演示
  - 可视化展示节点状态，按 Enter 逐步推进

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
go run ./exp6 -scenario all
go run ./exp6 -scenario normal
```

### 实验七（Raft 选主）

```powershell
go run ./exp7 -seed 7 -kill-after-ms 450
```

---

## 测试（可选）

```powershell
go test ./exp6_bonus/exp6_2pc
```
