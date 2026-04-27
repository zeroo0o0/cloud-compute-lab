# 裂土封疆演示实验 —— 代码与演示说明

本目录（`ch5/`）是一个独立 Go module，承载第 5 章的分布式系统演示代码。

目前已实现：

- 实验六：分布式事务 Two-Phase Commit（2PC）

---

## 目录结构

```text
ch5/
├── go.mod
├── README.md
├── 裂土封疆演示实验.md
├── cmd/
│   └── exp6/
│       └── two_phase_commit/
│           └── main.go
└── internal/
    └── exp6twopc/
        ├── twopc_engine.go
        ├── cinematic_renderer.go
        ├── scenario_normal_heist.go
        ├── scenario_a_reject.go
        ├── scenario_b_timeout.go
        ├── scenario_c_coord_crash_phase1.go
        ├── scenario_d_coord_crash_phase2.go
        └── demo_test.go
```

---

## 启动说明（仅保留剧情演绎模式）

```powershell
cd .\CourseCode\ch5

# 课堂推荐：完整剧情（normal -> Enter -> a -> Enter -> b -> Enter -> c -> Enter -> d）
go run ./cmd/exp6/two_phase_commit -scenario all -step-ms 900

# 可选：仅演示成功场景
go run ./cmd/exp6/two_phase_commit -scenario normal -step-ms 900
```

---

## 文件作用说明（代码分层）

- `cmd/exp6/two_phase_commit/main.go`
    - 启动入口，解析 `scenario/step-ms` 参数。

- `internal/exp6twopc/twopc_engine.go`
    - 2PC 通用基础设施：状态机、Coordinator/Worker、稳定存储、通用工具。
    - 只放“可复用底层能力”，不承载具体剧情细节。

- `internal/exp6twopc/cinematic_renderer.go`
    - 统一电影式渲染能力：逐字背景旁白、角色头像前缀、角色台词节奏控制。
    - 场景文件仅提供“剧情脚本数据”，具体渲染由此文件统一完成。

- `internal/exp6twopc/scenario_normal_heist.go`
    - 成功场景（normal）专用。
    - `runScenarioNormalCore`：纯 2PC 逻辑（中文注释标识）。
    - `renderScenarioNormalHeistDialogue`：剧情对话渲染（中文注释标识）。

- `internal/exp6twopc/scenario_a_reject.go`
    - 故障 A（拒票）专用。
    - `runScenarioACore`：纯 2PC 逻辑。
    - `renderScenarioADialogue`：剧情渲染。

- `internal/exp6twopc/scenario_b_timeout.go`
    - 故障 B（超时无响应）专用。
    - `runScenarioBCore`：纯 2PC 逻辑。
    - `renderScenarioBDialogue`：剧情渲染。

- `internal/exp6twopc/scenario_c_coord_crash_phase1.go`
    - 故障 C（Coordinator 第一阶段崩溃）专用。
    - `runScenarioCCore`：纯 2PC 逻辑。
    - `renderScenarioCDialogue`：剧情渲染。

- `internal/exp6twopc/scenario_d_coord_crash_phase2.go`
    - 故障 D（Coordinator 第二阶段崩溃 + 恢复重放）专用。
    - `runScenarioDCore`：纯 2PC 逻辑。
    - `renderScenarioDDialogue`：剧情渲染。

- `internal/exp6twopc/demo_test.go`
    - 最小回归测试集合（不是无用文件）。
    - `TestScenarioNormalCommit`：验证正常场景最终必须 `COMMIT`。
    - `TestScenarioDRecoveryReplaysCommit`：验证二阶段崩溃后可恢复并重放到 `COMMIT`。

## 运行入口

在 `ch5` 目录执行：

```powershell
go run ./cmd/exp6/two_phase_commit -scenario all -step-ms 900
go run ./cmd/exp6/two_phase_commit -scenario normal -step-ms 900
```

说明：`-scenario all` 采用**分阶段演示**，先展示 `normal` 正常流程并暂停，按 Enter 后再展示 `a/b/c/d` 故障流程。
说明：在 `-scenario all` 下，`a/b/c/d` 也改为**逐场景按 Enter 推进**（normal -> Enter -> a -> Enter -> b -> Enter -> c -> Enter -> d）。
说明：`-step-ms` 控制电影对白推进速度（默认 `650ms`）；设为 `0` 可一次性快速输出。
说明：在 `-scenario normal` 下，已改造为“纸钞屋印钞厂”角色对白（教授 / 东京丹佛 / 里约 / 柏林内罗毕），但底层仍严格遵循 2PC：`VOTE-REQ -> VOTE-COMMIT -> GLOBAL-COMMIT`。
说明：成功场景代码已拆分到 `scenario_normal_heist.go`，其中 `runScenarioNormalCore` 负责纯 2PC，`renderScenarioNormalHeistDialogue` 负责剧情对话渲染。

---

## 🎬 故事设定：《教授的完美协同实验》

### 🧭 背景世界观

在一个虚构世界中，存在一种数字与纸钞混合的货币：`Aurora（极光币）`。

皇家印钞厂不仅负责实体印刷，还掌握货币编号与流通合法性的认证系统。只有在印钞厂内完成并通过系统确认的一批印刷，才会被承认为“有效货币”。

### 🎭 核心人物

有一位极其理性、追求完美协作的策略设计者，人称：**教授（The Professor）**。

他并非追逐财富，而是在验证一个理论：

> 一群分布在不同位置的人，是否可以在完全同步、零冲突的情况下，完成一项复杂协作？

于是他设计了一场：**“零冲突协同演练”**。

### 🎯 目标（抽象化描述）

在不造成伤害、不破坏秩序的前提下，于极短时间内协同完成一次完整印刷流程，生成 **20 亿 Aurora（极光币）**。

关键约束：

- ❌ 不允许任何人提前行动
- ❌ 不允许局部成功、整体失败
- ✅ 必须所有人同步执行
- ✅ 必须“要么全部完成，要么完全不发生”

这本质上就是一次**全有或全无（原子性）**的协同实验。

### 🧩 行动结构（对应分布式节点）

教授将行动拆成多个独立节点：

- 柏林：流程控制（印刷节奏）
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

## 实验六实现要点

- 状态机：`INIT`、`WAIT`、`READY`、`COMMIT`、`ABORT`
- 每次状态迁移前后，均有“稳定存储”日志打印与落盘（`STATE_BEFORE/STATE_AFTER`）
- 场景覆盖：正常流程、拒票、超时、Coordinator 第一阶段崩溃、Coordinator 第二阶段崩溃并恢复
- 控制台包含教学提示：每个阶段会输出“提示（正在做什么）/预期（应观察到什么）”
- 每个关键阶段输出状态面板：`Coordinator`、`Worker-A`、`Worker-B` 当前状态
- 电影式可视化：统一输出“背景旁白 + 角色头像台词 + 旁白收束”，让协议步骤更贴近课堂叙事
- 架构分层：各场景文件坚持“核心2PC执行（Core）”与“剧情渲染（Dialogue）”分离

---

## 场景映射

- `normal`：正常演示（对应 P67）
- `a`：Worker 拒票（对应 P68）
- `b`：Worker 超时/无响应（对应 P70）
- `c`：Coordinator 第一阶段崩溃（对应 P72）
- `d`：Coordinator 第二阶段崩溃并恢复重放（对应 P73）

剧情化对照（引入警督——警方的指挥）：

- `normal`（协同成功）
    - 教授发起“零冲突协同演练”，所有小组均返回 YES，最终同步执行。

- `a`（拒票）
    - 正常场景成功后，教授发起“已印纸钞转运地下金库”的第二轮任务。
    - 东京/丹佛给出 YES，但里约判断监控盲区被警督锁定，条件不成立，返回 NO（VOTE-ABORT）。
    - 教授立即 `GLOBAL-ABORT`，避免局部推进。

- `b`（超时）
    - 承接 `a` 场景拒票后，教授调整方案并重新发起投票。
    - 警督持续干扰通信，里约组无法稳定返回投票结果。
    - 教授在 `WAIT` 超时后触发 `GLOBAL-ABORT`，防止事务悬挂。

- `c`（第一阶段崩溃）
    - 经历 A 的拒票与 B 的超时后，教授重排计划准备第三轮。
    - 警督升级无线压制，教授在发出正式投票请求前失联（崩溃）。
    - 各组在 `INIT` 超时后自动 `ABORT`，避免误提交。

- `d`（第二阶段崩溃）
    - 在 A/B/C 连续故障后，教授采用“先写盘再广播”的稳态方案。
    - 教授已写下 `DECISION=COMMIT`，却在广播前再次失联。
    - 各组卡在 `READY`；教授恢复后从稳定日志读取决议并重放 `GLOBAL-COMMIT`，事务最终闭环。

---

## 日志与恢复

运行后会生成目录：

- `./data/exp6_2pc/<scenario>/coordinator.log`
- `./data/exp6_2pc/<scenario>/worker_a.log`
- `./data/exp6_2pc/<scenario>/worker_b.log`

其中场景 `d` 会演示：

1. Coordinator 在收齐 `VOTE-COMMIT` 且写入 `DECISION=COMMIT` 后崩溃。
2. Worker 进入 `READY` 并阻塞。
3. 重启后的 Coordinator 执行 `loadPersistedState()`（代码中函数名为 `loadPersistedDecision`）恢复决议并重新广播 `GLOBAL-COMMIT`。

---

## 快速运行

```powershell
cd .\CourseCode\ch5
go run ./cmd/exp6/two_phase_commit -scenario all
```

你会先看到正常场景输出；按 Enter 后继续故障场景输出。运行后会在 `./data/exp6_2pc/` 下生成每个场景的稳定存储日志（Coordinator / Worker）。
在故障阶段中，`a/b/c/d` 也是按 Enter 逐个推进，便于课堂逐段讲解每一种故障与 2PC 反应。
