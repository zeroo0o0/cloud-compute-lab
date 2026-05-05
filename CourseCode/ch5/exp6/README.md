# 实验六：2PC 可视化流程演示（简化版）# 实验六：分布式事务 Two-Phase Commit（2PC）演示说明



本目录提供更易理解的 2PC 演示：**1 个协调者 + 3 个参与者** 的结构化可视化，并通过 **按 Enter 逐步推进** 的方式展示状态变化与事件。本目录对应第 5 章实验六，聚焦 2PC 的状态机、故障注入、稳定存储与恢复重放。



> 进阶版演示（包含完整日志与剧情渲染）已保留在 `exp6_bonus/`。---



---## 入口与代码分层



## 结构说明- 启动入口：`exp6/main.go`

  - 解析参数：`scenario`、`step-ms`、`data-dir`

- 上方：协调者服务（Service / Coordinator）

- 下方：三个数据库参与者（Worker）- 对外 API：`exp6/exp6_2pc/api.go`

- 每一步展示：当前状态 + 事件播报  - 对外暴露：`RunScenario`、`SetVisualStepDelay`、场景常量与状态常量



---- 核心协议：`exp6/exp6_2pc/core/engine.go`

  - 2PC 状态机：`INIT`、`WAIT`、`READY`、`COMMIT`、`ABORT`

## 启动方式  - Coordinator / Worker 模型

  - 稳定存储日志写入与恢复读取

在 `CourseCode/ch5` 目录执行：

- 场景编排：`exp6/exp6_2pc/scenario/`

```powershell  - `core_shared.go`：场景核心复用（统一 `runCore`、故障注入、步骤快照）

go run ./exp6 -scenario all  - `scenarios.go`：`normal/a/b/c/d` 场景配置与剧情渲染

```

- 渲染工具：`exp6/exp6_2pc/utils/renderer.go`

参数说明：  - 电影式对白与输出节奏控制



- `-scenario all`：依次播放正常 + 故障 A/B/C/D- 测试：`exp6/exp6_2pc/demo_test.go`

- `-scenario normal|a|b|c|d`：单场景演示  - `TestScenarioNormalCommit`

  - `TestScenarioDRecoveryReplaysCommit`

---

---

## 场景说明

## 启动方式

- `normal`：正常提交

- `a`：参与者拒票 → 全局回滚在 `CourseCode/ch5` 目录执行：

- `b`：参与者超时 → 全局回滚

- `c`：协调者在 Phase-1 崩溃 → 参与者超时回滚```powershell

- `d`：协调者写入决议后崩溃 → 恢复重放提交go run ./exp6 -scenario all -step-ms 900

go run ./exp6 -scenario normal -step-ms 900
```

参数说明：

- `-scenario all`：分阶段演示（`normal -> Enter -> a -> Enter -> b -> Enter -> c -> Enter -> d`）
- `-scenario normal|a|b|c|d`：单场景演示
- `-step-ms`：对白推进间隔；`0` 表示快速输出
- `-data-dir`：稳定存储日志目录（默认 `./data/exp6_2pc`）

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

- `./data/exp6_2pc/<scenario>/coordinator.log`
- `./data/exp6_2pc/<scenario>/worker_a.log`
- `./data/exp6_2pc/<scenario>/worker_b.log`

在场景 `d` 中可观察：

1. Coordinator 已写入 `DECISION=COMMIT` 后崩溃。
2. Worker 卡在 `READY` 等待决议。
3. Coordinator 恢复后读取日志并重放 `GLOBAL-COMMIT`，事务闭环。

---

## 故事化设定（教学叙事）

本实验采用“教授的完美协同实验”作为课堂叙事外壳，用以帮助理解 2PC 的原子提交语义：

- 先投票，再统一决议
- 要么全体提交，要么全体回滚
- 不允许局部推进

说明：故事为纯教学类比，不对应现实行为，不涉及暴力或违法实践。