package exp6_2pc

import (
	"time"
)

type scenarioACoreStep struct {
	CState TxState
	AState TxState
	BState TxState
}

type scenarioACoreResult struct {
	coord    *coordinator
	entry    *worker
	system   *worker
	decision TxState
	steps    []scenarioACoreStep
}

func runScenarioA(root string) (Report, error) {
	coreResult, err := runScenarioACore(root)
	if err != nil {
		return Report{}, err
	}
	renderScenarioADialogue(coreResult)
	return makeReport(ScenarioWorkerReject, coreResult.coord, coreResult.decision, coreResult.entry, coreResult.system), nil
}

// runScenarioACore：仅保留 2PC 核心逻辑（故障A：Worker-B 拒票），不输出剧情。
func runScenarioACore(root string) (scenarioACoreResult, error) {
	coord, entry, system, err := setup(ScenarioWorkerReject, root)
	if err != nil {
		return scenarioACoreResult{}, err
	}
	system.forceReject = true

	steps := []scenarioACoreStep{{CState: coord.state, AState: entry.state, BState: system.state}}

	if err := coord.logger.persist("START_TX", map[string]interface{}{"txID": storyTxID, "amount": storyTradeAmount}); err != nil {
		return scenarioACoreResult{}, err
	}
	if err := transitionState("Coordinator", coord.logger, &coord.state, StateWAIT, "发送 VOTE-REQ，等待全部投票"); err != nil {
		return scenarioACoreResult{}, err
	}
	steps = append(steps, scenarioACoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	okA, err := entry.onVoteReq(storyTxID, storyTradeAmount)
	if err != nil {
		return scenarioACoreResult{}, err
	}
	okB, err := system.onVoteReq(storyTxID, storyTradeAmount)
	if err != nil {
		return scenarioACoreResult{}, err
	}
	steps = append(steps, scenarioACoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	decision := StateABORT
	if okA && okB {
		decision = StateCOMMIT
	}
	if err := coord.logger.persist("DECISION", map[string]interface{}{"txID": storyTxID, "decision": decision}); err != nil {
		return scenarioACoreResult{}, err
	}

	coord.broadcastDecision(decision)
	if err := transitionState("Coordinator", coord.logger, &coord.state, StateABORT, "收到拒票，广播 GLOBAL-ABORT"); err != nil {
		return scenarioACoreResult{}, err
	}
	entry.awaitDecisionBlocking(150 * time.Millisecond)
	steps = append(steps, scenarioACoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	return scenarioACoreResult{coord: coord, entry: entry, system: system, decision: decision, steps: steps}, nil
}

// renderScenarioADialogue：仅做剧情化渲染（故障A），不承载 2PC 决策代码。
func renderScenarioADialogue(r scenarioACoreResult) {
	_ = r
	renderCinematicScene(
		"故障A：拒票（VOTE-ABORT）",
		[]string{
			"正常场景刚成功后，教授准备发起第二轮“转运已印纸钞到地下金库”的协同动作。",
			"该动作必须由入口组、系统组同时就绪，任一组否决则整体作废。",
		},
		[]cinematicLine{
			{Speaker: "教授", Text: "第二轮任务：封装后的纸钞，3 分钟内转运到地下金库。各组给投票。"},
			{Speaker: "东京/丹佛", Text: "入口走廊可控，运钞通道畅通，我们给 YES。"},
			{Speaker: "里约", Text: "我给 NO。警督已锁定两段监控盲区，转运窗口不成立。"},
			{Speaker: "旁白", Text: "出现单点否决，2PC 立即转入全局回滚路径。"},
			{Speaker: "教授", Text: "拒票成立，GLOBAL-ABORT。所有人留在原位，不做局部推进。"},
			{Speaker: "东京/丹佛", Text: "收到，取消转运动作，回到待命位。"},
			{Speaker: "里约", Text: "保持 ABORT，等待新窗口或新方案。"},
		},
	)
}
