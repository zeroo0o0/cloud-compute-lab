# Lab 3：多地图并行与多节点协同

> 总分：30 分  
> 前置要求：完成 Lab 1、Lab 2，理解 TCP 通信、JSON 序列化、Go 并发、互斥锁和基本文件持久化。

本实验在 Lab2 的基础上继续递进：从“单地图、单节点处理”的战斗程序，升级为“多地图并行 + 多服务节点协同”的分布式战场。

本实验不是单独写几个算法函数，而是要求把 **一致性哈希、Gossip、2PC、Raft** 融入游戏主流程：地图分片、节点故障、跨节点交易、主从切换都必须通过游戏场景体现出来。

---

## 一、实验目标

完成 Lab3 后，你需要理解并实现以下能力：

1. **多地图并行**
   - 系统中包含 `green`、`cave`、`ruins` 三张地图。
   - 每张地图独立维护玩家、NPC、宝物、障碍和地图版本。
   - 玩家可以在多张地图之间切换。

2. **多节点协同**
   - 系统内部模拟 `node-a`、`node-b`、`node-c` 三个逻辑服务节点。
   - 地图由不同节点承载，每张地图有主节点和副本节点。
   - 玩家操作需要根据当前地图路由到正确节点。

3. **分布式核心机制**
   - 使用一致性哈希决定地图的主节点和副本节点。
   - 使用 Gossip 维护节点成员状态。
   - 使用 2PC 保证跨节点战利品转移的原子性。
   - 使用 Raft 思路保证地图 owner 元数据变更经过多数提交。

4. **状态一致性与高可用**
   - 同一张地图同一时刻只能由一个主节点写入。
   - 世界 Boss 血量是全服共享状态。
   - 节点故障后，需要把副本提升为新主并修正玩家路由。
   - 冷数据和热数据需要分层保存。

---

## 二、学生目录结构

学生主要使用以下目录：

```text
Lab3/
├── README.md                         # 实验说明
├── student/                          # 学生代码目录
│   ├── go.mod
│   ├── protocol/
│   │   └── message.go                # 通信消息、地图快照、玩家视图等结构
│   ├── storage/
│   │   └── store.go                  # 冷热数据读写
│   ├── world/
│   │   └── world.go                  # 单张地图内的玩家、NPC、宝物、战斗逻辑
│   ├── cluster/
│   │   ├── cluster.go                # TODO C-1 ~ C-6：游戏主流程中的分布式协同
│   │   ├── hashring_support.go       # TODO A-1：地图主副本分配，一致性哈希支撑代码
│   │   ├── gossip_support.go         # TODO A-2：节点存活状态传播，Gossip 支撑代码
│   │   ├── twopc_support.go          # TODO A-3：跨节点交易原子性，2PC 支撑代码
│   │   ├── raft_support.go           # TODO A-4：元数据多数提交，Raft 支撑代码
│   │   └── checkpoint_support.go     # TODO C-4：checkpoint 一次性测试入口，属于 C-4
│   ├── cmd/
│   │   ├── server/
│   │   │   └── main.go               # 启动服务端
│   │   ├── client/
│   │   │   ├── main.go               # 启动客户端
│   │   │   ├── raw_mode_darwin.go    # macOS 即时按键支持
│   │   │   ├── raw_mode_linux.go     # Linux 即时按键支持
│   │   │   ├── raw_mode_windows.go   # Windows 即时按键支持
│   │   │   └── raw_mode_other.go     # 其他平台兜底实现
│   │   └── admin/
│   │       └── main.go               # 管理命令：状态、故障、恢复
│   └── data/
│       ├── cold/
│       │   └── users.json            # 冷数据：账号、密码、历史状态
│       └── hot/
│           └── sessions.json         # 热数据：在线会话
└── test/
    ├── autotest.go                   # 自动化测试入口，内嵌 6 个游戏场景测试
    ├── run_test.sh                   # macOS / Linux 测试脚本
    ├── run_test.bat                  # Windows 测试脚本
    ├── runner/
    │   └── main.go                   # 测试运行器
    └── .gocache/                     # 测试时自动生成，可忽略
```

你主要需要修改 `student/cluster/` 下标有 TODO 的文件。

这些 `*_support.go` 文件不是“脱离游戏的算法题”，而是为了避免 `cluster.go` 过长，把分布式机制拆出来：

1. `hashring_support.go` 被 `cluster.go` 用来决定地图 owner / replica。
2. `gossip_support.go` 被 `cluster.go` 用来维护节点 alive / suspect / dead 状态。
3. `twopc_support.go` 被 `TransferTreasures` 用来保证跨节点战利品交易原子性。
4. `raft_support.go` 被故障切换流程用来提交地图 owner 元数据变更。
5. `checkpoint_support.go` 只是 C-4 的一次性测试入口，和 `checkpointLoop` 属于同一个任务。

---

## 三、需要完成的 TODO

### A-1：一致性哈希主副本定位（4 分）

文件：`student/cluster/hashring_support.go`

需要完成：

1. `Ring.SetMembers`
2. `Ring.Locate`
3. `Ring.RebalancePlan`

要求：

1. 根据节点权重创建虚拟节点。
2. 对虚拟节点按哈希值排序。
3. 根据地图 key 找到主节点和不重复副本。
4. 节点变化时生成迁移计划，不能让所有地图都迁移。

游戏中的体现：地图 `green / cave / ruins` 的 owner 和 replica 必须由一致性哈希决定。

### A-2：Gossip 成员状态传播（4 分）

文件：`student/cluster/gossip_support.go`

需要完成：

1. `Table.Merge`
2. `Table.Targets`

要求：

1. 按 fanout 选择 Gossip 传播目标。
2. 不能选择自己，不能选择已经 dead 的节点。
3. 合并远端状态时，按 `incarnation -> heartbeat -> status` 判断新旧。
4. 节点长时间没有更新时，需要进入 suspect / dead 状态。

游戏中的体现：模拟节点故障后，故障节点需要在成员表中变为 `dead`。

### A-3：2PC 跨节点交易（4 分）

文件：`student/cluster/twopc_support.go`

需要完成：

1. `Coordinator.TransferWithParticipants`

要求：

1. 第一阶段向转出方和转入方执行 prepare。
2. 所有参与者 prepare 成功后，第二阶段执行 commit。
3. 任一参与者 prepare 失败，已经 prepare 的参与者必须 abort。
4. 不能出现只扣一方或只加一方的错误状态。

游戏中的体现：两个玩家在不同节点时，跨节点转移战利品必须满足原子性。

### A-4：Raft 元数据提交（4 分）

文件：`student/cluster/raft_support.go`

需要完成：

1. `Node.HandleRequestVote`
2. `Node.StartElection`
3. `Node.HandleAppendEntries`
4. `Node.Propose`

要求：

1. 候选节点发起选举，获得多数票后成为 leader。
2. follower 不能给旧任期或日志落后的候选人投票。
3. leader 追加元数据日志，并复制给其他节点。
4. 多数节点复制成功后，才能推进 commit index。

游戏中的体现：节点故障后，地图 owner 变更需要通过 Raft 元数据提交。

### C-1：世界 Boss 全服共享攻击（3 分）

文件：`student/cluster/cluster.go`

需要完成：

1. `AttackBoss`

要求：

1. 根据玩家所在地图找到世界 Boss 投影位置。
2. 玩家距离 Boss 太远时不能攻击。
3. 攻击时扣减全局 Boss 血量。
4. 多地图玩家看到的 Boss 血量必须一致。
5. Boss 被击杀后，需要记录终结玩家并结算奖励。

### C-2：跨地图切换与路由迁移（3 分）

文件：`student/cluster/cluster.go`

需要完成：

1. `SwitchMap`

要求：

1. 校验目标地图是否存在。
2. 玩家倒地时不能切换地图。
3. 从原地图节点移除玩家。
4. 把玩家加入目标地图主节点。
5. 更新在线会话中的 `MapID` 和 `NodeID`。

### C-3：冷热数据持久化（2 分）

文件：`student/cluster/cluster.go`

需要完成：

1. `persistSessionState`

要求：

1. 从在线会话中找到玩家当前地图和节点。
2. 从地图节点读取玩家最新运行状态。
3. 保存冷数据 `UserProfile`。
4. 保存热数据 `HotSession`。

### C-4：地图检查点与副本同步（2 分）

文件：`student/cluster/cluster.go`、`student/cluster/checkpoint_support.go`

需要完成：

1. `checkpointLoop`
2. `RunCheckpointOnce`

说明：

1. `checkpointLoop` 是服务端后台定时任务。
2. `RunCheckpointOnce` 是自动测试使用的一次性入口。
3. 两个函数都属于 C-4，不是两个独立任务。

要求：

1. 从地图主节点抓取 `MapCheckpoint`。
2. 把 checkpoint 写入热数据目录。
3. 把 checkpoint 同步给副本节点。
4. 节点故障时可以依靠 checkpoint 恢复地图。

### C-5：节点故障切换（2 分）

文件：`student/cluster/cluster.go`

需要完成：

1. `handleNodeFailure`

要求：

1. 找到故障节点承载的主地图。
2. 将该地图的副本节点提升为新主。
3. 更新 owner / replica 元数据。
4. 修正该地图上在线玩家的路由。
5. 广播故障切换事件。

### C-6：跨节点战利品转移（2 分）

文件：`student/cluster/cluster.go`

需要完成：

1. `TransferTreasures`

要求：

1. 找到转出玩家和转入玩家所在节点。
2. 构造两个 2PC 事务参与者。
3. 调用 2PC 协调器完成提交或回滚。
4. 交易完成后同步玩家冷热数据。

---

## 四、玩法操作

启动客户端后，先选择单机测试或连接指定网关，然后进行登录或注册。

游戏中常用按键：

| 按键 | 作用 |
| --- | --- |
| `W` / `A` / `S` / `D` | 上下左右移动 |
| `J` | 普通攻击附近 NPC 或玩家 |
| `K` | 使用药剂 |
| `B` | 攻击世界 Boss |
| `P` | 打开或关闭商店 |
| `1` | 购买药剂 |
| `2` | 强化武器 |
| `M` | 切换地图 |
| `Q` | 退出游戏 |

说明：战斗操作是即时按键，不需要输入回车。

---

## 五、运行方式

在一个终端启动服务端：

```bash
cd Lab3/student
go run ./cmd/server
```

在另一个终端启动客户端：

```bash
cd Lab3/student
go run ./cmd/client
```

查看集群状态：

```bash
cd Lab3/student
go run ./cmd/admin 状态
```

模拟节点故障：

```bash
cd Lab3/student
go run ./cmd/admin 故障 node-a
```

恢复节点：

```bash
cd Lab3/student
go run ./cmd/admin 恢复 node-a
```

---

## 六、测试方式

macOS / Linux：

```bash
cd Lab3/test
./run_test.sh
```

Windows：

```bat
cd Lab3\test
run_test.bat
```

测试说明：

1. 测试默认检查 `student/` 目录。
2. 测试不是孤立算法单测，而是从游戏场景进入。
3. 如果某个 TODO 没完成，测试会输出类似 `[Lab3-A1]`、`[Lab3-C3]` 的定位信息。

当前测试包含 6 个场景：

1. 注册登录、拓扑发布与一致性哈希分片。
2. 多地图切换与节点路由。
3. 世界 Boss 跨地图共享血量。
4. 跨节点战利品转移与 2PC 回滚。
5. 地图检查点与冷热数据恢复。
6. Gossip 故障发现与 Raft 主从切换。

---

## 七、调试建议

1. 如果一开始测试就失败在 `[Lab3-A1]`，说明一致性哈希没有完成，集群无法完成地图分片初始化。
2. 如果交易测试失败，重点检查 2PC 的 prepare / commit / abort 顺序。
3. 如果故障切换测试失败，重点检查 Gossip 状态、Raft 元数据提交、owner / replica 更新和玩家路由修正。
4. 如果重登后状态不对，重点检查冷数据 `UserProfile` 和热数据 `HotSession` 是否都被保存。
5. 如果世界 Boss 血量不一致，说明 Boss 状态被错误地放进单张地图或单个节点中。

---

## 八、验收关注点

课堂汇报时，需要重点解释：

1. 一致性哈希为什么适合做地图分片。
2. Gossip 为什么适合做节点健康状态传播。
3. 2PC 如何保证跨节点交易原子性。
4. Raft 为什么要求多数提交后才能修改 owner。
5. 地图切换为什么不只是修改一个 `MapID`。
6. 世界 Boss 为什么必须是全局共享状态。
7. 节点故障后为什么要修正在线玩家路由。
8. 冷数据和热数据分别保存什么。
