package exp6_2pc

import (
	"time"
)

type scenarioDCoreStep struct {
	CState TxState
	AState TxState
	BState TxState
}

type scenarioDCoreResult struct {
	coord    *coordinator
	entry    *worker
	system   *worker
	decision TxState
	steps    []scenarioDCoreStep
}

func runScenarioD(root string) (Report, error) {
	coreResult, err := runScenarioDCore(root)
	if err != nil {
		return Report{}, err
	}
	renderScenarioDDialogue(coreResult)
	return makeReport(ScenarioCoordCrashPhase2, coreResult.coord, coreResult.decision, coreResult.entry, coreResult.system), nil
}

// runScenarioDCore：仅保留 2PC 核心逻辑（故障D：二阶段崩溃 + 恢复重放），不输出剧情。
func runScenarioDCore(root string) (scenarioDCoreResult, error) {
	coord, entry, system, err := setup(ScenarioCoordCrashPhase2, root)
	if err != nil {
		return scenarioDCoreResult{}, err
	}

	steps := []scenarioDCoreStep{{CState: coord.state, AState: entry.state, BState: system.state}}

	if err := coord.logger.persist("START_TX", map[string]interface{}{"txID": storyTxID, "amount": storyTradeAmount}); err != nil {
		return scenarioDCoreResult{}, err
	}
	if err := transitionState("Coordinator", coord.logger, &coord.state, StateWAIT, "发送 VOTE-REQ，等待全部投票"); err != nil {
		return scenarioDCoreResult{}, err
	}
	steps = append(steps, scenarioDCoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	okA, err := entry.onVoteReq(storyTxID, storyTradeAmount)
	if err != nil {
		return scenarioDCoreResult{}, err
	}
	okB, err := system.onVoteReq(storyTxID, storyTradeAmount)
	if err != nil {
		return scenarioDCoreResult{}, err
	}
	steps = append(steps, scenarioDCoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	decision := StateABORT
	if okA && okB {
		decision = StateCOMMIT
	}
	if err := coord.logger.persist("DECISION", map[string]interface{}{"txID": storyTxID, "decision": decision}); err != nil {
		return scenarioDCoreResult{}, err
	}
	if err := coord.logger.persist("CRASH", map[string]interface{}{"phase": "after_decision_before_broadcast", "txID": storyTxID, "decision": decision}); err != nil {
		return scenarioDCoreResult{}, err
	}
	steps = append(steps, scenarioDCoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	entry.awaitDecisionBlocking(220 * time.Millisecond)
	system.awaitDecisionBlocking(220 * time.Millisecond)
	steps = append(steps, scenarioDCoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	recoverCoord := newCoordinator(coord.logger, []*worker{entry, system})
	recoveredDecision, err := recoverCoord.recoverAndReplay()
	if err != nil {
		return scenarioDCoreResult{}, err
	}
	entry.awaitDecisionBlocking(300 * time.Millisecond)
	system.awaitDecisionBlocking(300 * time.Millisecond)
	steps = append(steps, scenarioDCoreStep{CState: recoverCoord.state, AState: entry.state, BState: system.state})

	if recoveredDecision != "" {
		decision = recoveredDecision
	}
	return scenarioDCoreResult{coord: recoverCoord, entry: entry, system: system, decision: decision, steps: steps}, nil
}

// renderScenarioDDialogue：仅做剧情化渲染（故障D），不承载 2PC 决策代码。
func renderScenarioDDialogue(r scenarioDCoreResult) {
	_ = r
	renderCinematicScene(
		"故障D：决议已写盘，但广播前失联",
		[]string{
			"经历 A 的拒票、B 的超时、C 的一阶段失联后，教授改用更稳的本地写盘策略。",
			"这一轮他收齐所有 YES，并先把 DECISION=COMMIT 写入稳定日志。",
			"但在广播 GLOBAL-COMMIT 前，警督再次干扰链路，系统进入最危险窗口。",
			"2PC 规则要求：READY 节点必须等待最终决议，任何人不得擅自提交或回滚。",
		},
		[]cinematicLine{
			{Speaker: "教授", Text: "收齐 YES，先落盘 DECISION=COMMIT，再统一广播。"},
			{Speaker: "旁白", Text: "决议刚写入，广播尚未发出，主控信号再次中断。"},
			{Speaker: "东京/丹佛", Text: "入口组停在 READY，按协议等待，不擅自推进。"},
			{Speaker: "里约", Text: "系统组也停在 READY，保持阻塞。"},
			{Speaker: "警督", Text: "这次你们会被卡死在中间态。"},
			{Speaker: "旁白", Text: "教授恢复上线，读取稳定日志，确认旧决议是 COMMIT。"},
			{Speaker: "教授", Text: "按日志重放：GLOBAL-COMMIT。所有组继续执行。"},
			{Speaker: "东京/丹佛", Text: "收到，切换 COMMIT。"},
			{Speaker: "里约", Text: "收到，切换 COMMIT。"},
			{Speaker: "旁白", Text: "事务闭环：即便二阶段崩溃，也通过恢复重放达成一致提交。"},
		},
	)
}
