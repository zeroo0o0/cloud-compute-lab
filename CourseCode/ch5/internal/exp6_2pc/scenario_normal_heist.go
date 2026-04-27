package exp6_2pc

import (
	"os"
	"path/filepath"
	"time"
)

type normalHeistCoreStep struct {
	CState  TxState
	AState  TxState
	BState  TxState
	C2State TxState
}

type normalHeistCoreResult struct {
	coord    *coordinator
	entry    *worker
	system   *worker
	printing *worker
	decision TxState
	steps    []normalHeistCoreStep
}

func runScenarioNormal(root string) (Report, error) {
	coreResult, err := runScenarioNormalCore(root)
	if err != nil {
		return Report{}, err
	}

	renderScenarioNormalHeistDialogue(coreResult)
	return makeReport(ScenarioNormal, coreResult.coord, coreResult.decision, coreResult.entry, coreResult.system, coreResult.printing), nil
}

// runScenarioNormalCore 仅包含 2PC 核心语义，不做剧情输出。
func runScenarioNormalCore(root string) (normalHeistCoreResult, error) {
	scenarioDir := filepath.Join(root, string(ScenarioNormal))
	if err := os.RemoveAll(scenarioDir); err != nil {
		return normalHeistCoreResult{}, err
	}
	if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
		return normalHeistCoreResult{}, err
	}

	entry := newWorker("Worker-A", 100, newStableLogger("Worker-A", filepath.Join(scenarioDir, "worker_a.log")))
	system := newWorker("Worker-B", 100, newStableLogger("Worker-B", filepath.Join(scenarioDir, "worker_b.log")))
	printing := newWorker("Worker-C", 100, newStableLogger("Worker-C", filepath.Join(scenarioDir, "worker_c.log")))
	coord := newCoordinator(newStableLogger("Coordinator", filepath.Join(scenarioDir, "coordinator.log")), []*worker{entry, system, printing})

	if err := entry.beginTx(storyTxID); err != nil {
		return normalHeistCoreResult{}, err
	}
	if err := system.beginTx(storyTxID); err != nil {
		return normalHeistCoreResult{}, err
	}
	if err := printing.beginTx(storyTxID); err != nil {
		return normalHeistCoreResult{}, err
	}

	steps := []normalHeistCoreStep{{CState: coord.state, AState: entry.state, BState: system.state, C2State: printing.state}}

	if err := coord.logger.persist("START_TX", map[string]interface{}{"txID": storyTxID, "amount": storyTradeAmount}); err != nil {
		return normalHeistCoreResult{}, err
	}
	if err := transitionState("Coordinator", coord.logger, &coord.state, StateWAIT, "发送 VOTE-REQ，等待全部投票"); err != nil {
		return normalHeistCoreResult{}, err
	}
	steps = append(steps, normalHeistCoreStep{CState: coord.state, AState: entry.state, BState: system.state, C2State: printing.state})

	okA, err := entry.onVoteReq(storyTxID, storyTradeAmount)
	if err != nil {
		return normalHeistCoreResult{}, err
	}
	okB, err := system.onVoteReq(storyTxID, storyTradeAmount)
	if err != nil {
		return normalHeistCoreResult{}, err
	}
	okC, err := printing.onVoteReq(storyTxID, storyTradeAmount)
	if err != nil {
		return normalHeistCoreResult{}, err
	}
	steps = append(steps, normalHeistCoreStep{CState: coord.state, AState: entry.state, BState: system.state, C2State: printing.state})

	decision := StateABORT
	if okA && okB && okC {
		decision = StateCOMMIT
	}
	if err := coord.logger.persist("DECISION", map[string]interface{}{"txID": storyTxID, "decision": decision}); err != nil {
		return normalHeistCoreResult{}, err
	}
	steps = append(steps, normalHeistCoreStep{CState: coord.state, AState: entry.state, BState: system.state, C2State: printing.state})

	coord.broadcastDecision(decision)
	if decision == StateCOMMIT {
		if err := transitionState("Coordinator", coord.logger, &coord.state, StateCOMMIT, "广播 GLOBAL-COMMIT"); err != nil {
			return normalHeistCoreResult{}, err
		}
	} else {
		if err := transitionState("Coordinator", coord.logger, &coord.state, StateABORT, "广播 GLOBAL-ABORT"); err != nil {
			return normalHeistCoreResult{}, err
		}
	}
	steps = append(steps, normalHeistCoreStep{CState: coord.state, AState: entry.state, BState: system.state, C2State: printing.state})

	entry.awaitDecisionBlocking(150 * time.Millisecond)
	system.awaitDecisionBlocking(150 * time.Millisecond)
	printing.awaitDecisionBlocking(150 * time.Millisecond)
	steps = append(steps, normalHeistCoreStep{CState: coord.state, AState: entry.state, BState: system.state, C2State: printing.state})

	return normalHeistCoreResult{
		coord:    coord,
		entry:    entry,
		system:   system,
		printing: printing,
		decision: decision,
		steps:    steps,
	}, nil
}

// renderScenarioNormalHeistDialogue 仅负责剧情渲染，不承载 2PC 决策逻辑。
func renderScenarioNormalHeistDialogue(r normalHeistCoreResult) {
	_ = r
	renderCinematicScene(
		"正常场景：教授的完美协同实验（2PC 成功提交）",
		[]string{
			"Aurora 印钞厂内，教授准备验证“零冲突同步协作”理论。",
			"所有小组先确认可执行，再等待统一指令。",
		},
		[]cinematicLine{
			{Speaker: "教授", Text: "各组汇报状态。没有我的最终指令，任何人不得行动。"},
			{Speaker: "东京/丹佛", Text: "入口已就位，准备完成。我们可提交（VOTE-COMMIT）。"},
			{Speaker: "里约", Text: "系统已切换，准备完成。可提交（VOTE-COMMIT）。"},
			{Speaker: "柏林/内罗毕", Text: "印刷链路与油墨全部就绪。可提交（VOTE-COMMIT）。"},
			{Speaker: "旁白", Text: "教授收齐全部 YES，写入决议 DECISION=COMMIT。"},
			{Speaker: "教授", Text: "所有人确认完毕，let's ↘ go↗~（GLOBAL-COMMIT）"},
			{Speaker: "东京/丹佛", Text: "收到，执行入口控制。"},
			{Speaker: "里约", Text: "收到，执行系统控制。"},
			{Speaker: "柏林/内罗毕", Text: "收到，执行印刷流程。"},
			{Speaker: "旁白", Text: "事务闭环成功：20 亿 Aurora 记账生效（COMMIT）。"},
		},
	)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
