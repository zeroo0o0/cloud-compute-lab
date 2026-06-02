# 实验七：Raft 领导者选举（多进程模拟）

本目录对应第 5 章实验七，通过启动多个独立进程模拟分布式环境，演示 Raft 领导者选举与故障转移。

---

## 与实验六的区别

| | 实验六 | 实验七 |
|--|--------|--------|
| 模拟方式 | 单进程 + goroutine | 多进程 + HTTP RPC |
| 通信方式 | channel | 网络 HTTP JSON-RPC |
| 节点隔离 | 共享内存 | 独立进程、独立地址空间 |
| 启动方式 | 一条命令启动全部 | 每个节点在独立终端启动 |

---

## 文件结构

```
exp7/
├── main.go   # 入口：解析参数、创建节点、启动 HTTP server
├── node.go   # Node 结构体、Raft 状态机、选举逻辑
├── rpc.go    # RPC 消息结构体、HTTP handler 和 client
└── README.md
```

---

## 启动方式

打开 **3 个终端**，在项目根目录下分别执行：

```powershell
# 终端 1
go run -C ./CourseCode/ch5/exp7 . -id 1 -port 8001 -peers localhost:8002,localhost:8003

# 终端 2
go run -C ./CourseCode/ch5/exp7 . -id 2 -port 8002 -peers localhost:8001,localhost:8003

# 终端 3
go run -C ./CourseCode/ch5/exp7 . -id 3 -port 8003 -peers localhost:8001,localhost:8002
```

参数说明：

| 参数 | 说明 |
|------|------|
| `-id` | 节点 ID（1, 2, 3） |
| `-port` | HTTP 监听端口 |
| `-peers` | 逗号分隔的其他节点地址 |

---

## 模拟 Leader 崩溃

当选 Leader 后，在该终端按 **Ctrl+C** 终止进程。剩余两个节点会因收不到心跳而超时，自动发起新一轮选举，产生新 Leader（Term = 2）。

---

## 预期输出示例

### 正常选举

```
[Node 1][Term 0][Follower] started at :8001, peers=[localhost:8002 localhost:8003]
[Node 2][Term 0][Follower] started at :8002, peers=[localhost:8001 localhost:8003]
[Node 3][Term 0][Follower] started at :8003, peers=[localhost:8001 localhost:8002]

... 150~300ms 后 ...

[Node 2][Term 1][Candidate] election timeout, start election
[Node 1][Term 1][Follower] voted for Node 2
[Node 3][Term 1][Follower] voted for Node 2
[Node 2][Term 1][Candidate] vote granted by localhost:8001
[Node 2][Term 1][Candidate] vote granted by localhost:8003
[Node 2][Term 1][Leader] elected as Leader with 2 votes
[Node 2][Term 1][Leader] send heartbeat to localhost:8001
[Node 2][Term 1][Leader] send heartbeat to localhost:8003
[Node 1][Term 1][Follower] received heartbeat from Leader 2, reset election timer
[Node 3][Term 1][Follower] received heartbeat from Leader 2, reset election timer
```

### Leader 崩溃后故障转移

```
# 用户在 Node 2 的终端按 Ctrl+C

[Node 1][Term 1][Follower] election timeout, start election
[Node 1][Term 2][Candidate] election timeout, start election
[Node 3][Term 2][Follower] voted for Node 1
[Node 1][Term 2][Candidate] vote granted by localhost:8003
[Node 1][Term 2][Leader] elected as Leader with 2 votes
```

---

## 实验要点

- 每个节点作为独立进程运行，通过 HTTP JSON-RPC 通信
- Follower 随机选举超时 150~300ms，超时后变为 Candidate
- Candidate 向所有 peer 发送 RequestVote RPC 拉票
- 获得多数票（3 节点中至少 2 票）的 Candidate 当选 Leader
- Leader 每 50ms 发送 AppendEntries 心跳维持领导权
- 收到合法心跳的 Follower 重置选举超时，不发起选举
- 手动 Kill Leader 后，剩余节点自动恢复并选举新 Leader
- HTTP 客户端超时 100ms，连接失败不崩溃
