package exp6_2pc

import (
	"time"
)

type scenarioBCoreStep struct {
	CState TxState
	AState TxState
	BState TxState
}

type scenarioBCoreResult struct {
	coord    *coordinator
	entry    *worker
	system   *worker
	decision TxState
	steps    []scenarioBCoreStep
}

func runScenarioB(root string) (Report, error) {
	coreResult, err := runScenarioBCore(root)
	if err != nil {
		return Report{}, err
	}
	renderScenarioBDialogue(coreResult)
	return makeReport(ScenarioWorkerTimeout, coreResult.coord, coreResult.decision, coreResult.entry, coreResult.system), nil
}

// runScenarioBCore：仅保留 2PC 核心逻辑（故障B：Worker-B 超时无响应），不输出剧情。
func runScenarioBCore(root string) (scenarioBCoreResult, error) {
	coord, entry, system, err := setup(ScenarioWorkerTimeout, root)
	if err != nil {
		return scenarioBCoreResult{}, err
	}
	system.forceNoResp = true

	steps := []scenarioBCoreStep{{CState: coord.state, AState: entry.state, BState: system.state}}

	if err := coord.logger.persist("START_TX", map[string]interface{}{"txID": storyTxID, "amount": storyTradeAmount}); err != nil {
		return scenarioBCoreResult{}, err
	}
	if err := transitionState("Coordinator", coord.logger, &coord.state, StateWAIT, "发送 VOTE-REQ，等待全部投票"); err != nil {
		return scenarioBCoreResult{}, err
	}
	steps = append(steps, scenarioBCoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	okA, err := entry.onVoteReq(storyTxID, storyTradeAmount)
	if err != nil {
		return scenarioBCoreResult{}, err
	}
	_, err = system.onVoteReq(storyTxID, storyTradeAmount)
	if err == nil {
		// 理论上超时场景应返回错误；此分支仅用于防御性保护。
	}
	steps = append(steps, scenarioBCoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	decision := StateABORT
	if okA {
		decision = StateABORT
	}
	if err := coord.logger.persist("DECISION", map[string]interface{}{"txID": storyTxID, "decision": decision}); err != nil {
		return scenarioBCoreResult{}, err
	}

	coord.broadcastDecision(decision)
	if err := transitionState("Coordinator", coord.logger, &coord.state, StateABORT, "WAIT 超时，广播 GLOBAL-ABORT"); err != nil {
		return scenarioBCoreResult{}, err
	}
	entry.awaitDecisionBlocking(150 * time.Millisecond)
	steps = append(steps, scenarioBCoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	return scenarioBCoreResult{coord: coord, entry: entry, system: system, decision: decision, steps: steps}, nil
}

// renderScenarioBDialogue：仅做剧情化渲染（故障B），不承载 2PC 决策代码。
func renderScenarioBDialogue(r scenarioBCoreResult) {
	_ = r
	renderCinematicScene(
		"故障B：超时无响应（TIMEOUT）",
		[]string{
			"A 场景里约拒票后，教授临时调整路线，准备重新发起一次更保守的协同。",
			"警督同步升级了对通信骨干的干扰，系统组链路出现严重抖动。",
			"2PC 规则要求：在 WAIT 阶段超时未收齐投票，必须全局回滚。",
		},
		[]cinematicLine{
			{Speaker: "教授", Text: "上一轮否决我接受，这一轮按新窗口再投一次。各组回复状态。"},
			{Speaker: "东京/丹佛", Text: "入口组仍可执行，给 YES。"},
			{Speaker: "里约", Text: "...（链路抖动）... 信道丢包 ... 无法稳定回传 ..."},
			{Speaker: "警督", Text: "继续压制通信，让他们收不到完整投票。"},
			{Speaker: "旁白", Text: "WAIT 计时器归零，教授判定超时分支成立。"},
			{Speaker: "教授", Text: "未收齐投票，GLOBAL-ABORT。全员回滚，准备下一轮。"},
			{Speaker: "东京/丹佛", Text: "收到，入口组回到待命位。"},
		},
	)
}
