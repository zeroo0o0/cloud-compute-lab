# 实验六：分布式事务 Two-Phase Commit（2PC）演示说明

本目录对应第 5 章实验六，聚焦 2PC 的状态机、故障注入、稳定存储与恢复重放。

---

## 入口与代码分层

- 启动入口：`exp6_bonus/main.go`
  - 解析参数：`scenario`、`step-ms`、`data-dir`

- 对外 API：`exp6_bonus/exp6_2pc/api.go`
  - 对外暴露：`RunScenario`、`SetVisualStepDelay`、场景常量与状态常量

- 核心协议：`exp6_bonus/exp6_2pc/core/engine.go`
  - 2PC 状态机：`INIT`、`WAIT`、`READY`、`COMMIT`、`ABORT`
  - Coordinator / Worker 模型
  - 稳定存储日志写入与恢复读取

- 场景编排：`exp6_bonus/exp6_2pc/scenario/`
  - `core_shared.go`：场景核心复用（统一 `runCore`、故障注入、步骤快照）
  - `scenarios.go`：`normal/a/b/c/d` 场景配置与剧情渲染

- 渲染工具：`exp6_bonus/exp6_2pc/utils/renderer.go`
  - 电影式对白与输出节奏控制

- 测试：`exp6_bonus/exp6_2pc/demo_test.go`
  - `TestScenarioNormalCommit`
  - `TestScenarioDRecoveryReplaysCommit`

---

## 启动方式

在 `CourseCode/ch5` 目录执行：

```powershell
go run ./exp6_bonus -scenario all -step-ms 900
go run ./exp6_bonus -scenario normal -step-ms 900
```

参数说明：

- `-scenario all`：分阶段演示（`normal -> Enter -> a -> Enter -> b -> Enter -> c -> Enter -> d`）
- `-scenario normal|a|b|c|d`：单场景演示
- `-step-ms`：对白推进间隔；`0` 表示快速输出
- `-data-dir`：稳定存储日志目录（默认 `./exp6_bonus/data/exp6_2pc`）

---

## 场景映射

- `normal`：正常流程（P67）
- `a`：Worker 拒票（P68）
- `b`：Worker 超时/无响应（P70）
- `c`：Coordinator 第一阶段崩溃（P72）
- `d`：Coordinator 第二阶段崩溃后恢复重放（P73）

---

## 实验要点

- 严格的有限状态机迁移
- 关键迁移前后稳定存储日志（`STATE_BEFORE/STATE_AFTER`）
- 故障注入覆盖拒票、超时、协调者崩溃
- D 场景通过日志恢复并重放全局决议，验证最终一致性

---

## 日志与恢复观察

运行后会生成：

- `./exp6_bonus/data/exp6_2pc/<scenario>/coordinator.log`
- `./exp6_bonus/data/exp6_2pc/<scenario>/worker_a.log`
- `./exp6_bonus/data/exp6_2pc/<scenario>/worker_b.log`

在场景 `d` 中可观察：

1. Coordinator 已写入 `DECISION=COMMIT` 后崩溃。
2. Worker 卡在 `READY` 等待决议。
3. Coordinator 恢复后读取日志并重放 `GLOBAL-COMMIT`，事务闭环。

---

## 🎬 故事设定：《教授的完美协同实验》

### 🧭 背景世界观

在我们的游戏世界中，存在一种虚拟货币：`Aurora（极光币）`。

有一个金币副本，地图上有一个特殊的基地，皇家印钞厂。皇家印钞厂不仅负责实体印刷，还掌握货币编号与流通合法性的认证系统。只有在印钞厂内完成并通过系统确认的一批印刷，才会被承认为“有效货币”。

### 🎭 核心人物

有一位极其理性、追求完美协作的策略设计者，人称：**教授（The Professor）**。

他并非追逐财富，而是在验证一个理论：

> 一群分布在不同位置的人，是否可以在完全同步、零冲突的情况下，完成一项复杂协作？

于是他设计了一场：**“零冲突协同演练”**。

### 🎯 目标（抽象化描述）

在不造成伤害、不破坏秩序的前提下，于极短时间内协同完成一次完整胁持皇家印钞厂并完成印刷流程，生成 **20 亿 Aurora（极光币）**。

关键约束：

- ❌ 不允许任何人提前行动
- ❌ 不允许局部成功、整体失败
- ✅ 必须所有人同步执行
- ✅ 必须“要么全部完成，要么完全不发生”

这本质上就是一次**全有或全无（原子性）**的协同实验。

### 🧩 行动结构（对应分布式节点）

教授将行动拆成多个独立节点：

- 柏林/内罗毕：流程控制（胁持印刷厂，控制印刷节奏）
- 里约：系统控制（认证系统与监控切换）
- 东京 / 丹佛：入口执行（进入与外部隔离）
- 其他成员：各自负责关键点位

每个人都只能控制自己负责的那一部分。

### ⚙️ 核心协作规则（2PC 映射关键）

教授制定绝对规则：

> 没有我的最终指令，任何人不得行动。

并强调：

> 你们必须先确认自己可以完成任务；但在我下令前，谁都不能真正开始。

这对应到 2PC 的语义就是：

- 第一阶段先“询问可否提交”（`VOTE-REQ`）
- 所有参与者确认后才进入统一提交（`GLOBAL-COMMIT`）

### 🎬 戏剧核心：同步瞬间

教授追求的不是“慢慢完成”，而是一个瞬间：

所有节点同时从“准备状态（READY）”切换到“执行状态（COMMIT）”。

若任一条件不满足，则整次行动必须作废（`ABORT`）。

### 💡 实验本质

这个故事本质是在讲一个分布式协作问题：

- 一致性：所有节点做出同一个最终决定
- 原子性：要么全部发生，要么全部不发生
- 无冲突执行：无提前、无掉队、无分叉

### ⚠️ 安全与价值观说明

本故事为虚构协同实验设定：

- 不涉及真实犯罪行为
- 不包含任何暴力或伤害情节
- 所有角色行为仅用于说明分布式系统中的协调机制
- “印刷货币”仅作为抽象目标，用于类比“事务执行结果”

---