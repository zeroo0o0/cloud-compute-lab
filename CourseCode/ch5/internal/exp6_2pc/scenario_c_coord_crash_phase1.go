package exp6_2pc

import (
	"time"
)

type scenarioCCoreStep struct {
	CState TxState
	AState TxState
	BState TxState
}

type scenarioCCoreResult struct {
	coord    *coordinator
	entry    *worker
	system   *worker
	decision TxState
	steps    []scenarioCCoreStep
}

func runScenarioC(root string) (Report, error) {
	coreResult, err := runScenarioCCore(root)
	if err != nil {
		return Report{}, err
	}
	renderScenarioCDialogue(coreResult)
	return makeReport(ScenarioCoordCrashPhase1, coreResult.coord, coreResult.decision, coreResult.entry, coreResult.system), nil
}

// runScenarioCCore：仅保留 2PC 核心逻辑（故障C：Coordinator 一阶段崩溃），不输出剧情。
func runScenarioCCore(root string) (scenarioCCoreResult, error) {
	coord, entry, system, err := setup(ScenarioCoordCrashPhase1, root)
	if err != nil {
		return scenarioCCoreResult{}, err
	}

	steps := []scenarioCCoreStep{{CState: coord.state, AState: entry.state, BState: system.state}}

	if err := coord.logger.persist("START_TX", map[string]interface{}{"txID": storyTxID, "amount": storyTradeAmount}); err != nil {
		return scenarioCCoreResult{}, err
	}
	if err := coord.logger.persist("CRASH", map[string]interface{}{"phase": "before_vote_req", "txID": storyTxID}); err != nil {
		return scenarioCCoreResult{}, err
	}
	steps = append(steps, scenarioCCoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	go entry.abortIfInitTimeout(250*time.Millisecond, storyTxID)
	go system.abortIfInitTimeout(250*time.Millisecond, storyTxID)
	time.Sleep(350 * time.Millisecond)
	steps = append(steps, scenarioCCoreStep{CState: coord.state, AState: entry.state, BState: system.state})

	return scenarioCCoreResult{coord: coord, entry: entry, system: system, decision: StateABORT, steps: steps}, nil
}

// renderScenarioCDialogue：仅做剧情化渲染（故障C），不承载 2PC 决策代码。
func renderScenarioCDialogue(r scenarioCCoreResult) {
	_ = r
	renderCinematicScene(
		"故障C：第一阶段崩溃（Coordinator Crash Before Vote）",
		[]string{
			"在 A 场景拒票、B 场景超时之后，教授重排线路，准备第三次协同尝试。",
			"警督加大无线压制，教授在发出正式 VOTE-REQ 之前突然失联。",
			"2PC 规则要求：若参与者长期停在 INIT 且未收到投票请求，只能超时自 ABORT。",
		},
		[]cinematicLine{
			{Speaker: "教授", Text: "第三轮开始前最后确认：我将发起 VOTE-REQ。"},
			{Speaker: "警督", Text: "现在切断主控信道，别让他把口令发出去。"},
			{Speaker: "旁白", Text: "主控链路瞬断，教授在发包前离线。"},
			{Speaker: "东京/丹佛", Text: "我们还在 INIT，没收到投票请求，先按协议等待。"},
			{Speaker: "里约", Text: "系统组同样 INIT；超时计时器已启动。"},
			{Speaker: "旁白", Text: "等待窗口耗尽：各组按规则从 INIT 自动切到 ABORT。"},
		},
	)
}
