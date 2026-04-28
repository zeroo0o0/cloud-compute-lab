package scenario

import (
	"fmt"
	"time"

	"ch5/internal/exp6_2pc/core"
	"ch5/internal/exp6_2pc/utils"
)

// RunScenario 是场景层统一入口：根据场景编号路由到对应的“核心执行 + 动态渲染”。
func RunScenario(s core.Scenario, rootDataDir string) (core.Report, error) {
	switch s {
	case core.ScenarioNormal:
		return runScenarioNormal(rootDataDir)
	case core.ScenarioWorkerReject:
		return runScenarioA(rootDataDir)
	case core.ScenarioWorkerTimeout:
		return runScenarioB(rootDataDir)
	case core.ScenarioCoordCrashPhase1:
		return runScenarioC(rootDataDir)
	case core.ScenarioCoordCrashPhase2:
		return runScenarioD(rootDataDir)
	default:
		return core.Report{}, fmt.Errorf("unknown scenario: %s", s)
	}
}

// normal：三节点完整成功路径。
func runScenarioNormal(root string) (core.Report, error) {
	r, err := runCore(root, coreSpec{scenario: core.ScenarioNormal, workerCount: 3, phase1Timeout: 250 * time.Millisecond, decisionWait: 150 * time.Millisecond})
	if err != nil {
		return core.Report{}, err
	}
	renderScenarioNormal(r)
	return core.MakeReport(core.ScenarioNormal, r.coord, r.decision, r.entry, r.system, r.printing), nil
}

// A：Worker-B 拒票导致全局 ABORT。
func runScenarioA(root string) (core.Report, error) {
	r, err := runCore(root, coreSpec{scenario: core.ScenarioWorkerReject, workerCount: 2, forceRejectOnB: true, phase1Timeout: 250 * time.Millisecond, decisionWait: 150 * time.Millisecond})
	if err != nil {
		return core.Report{}, err
	}
	renderScenarioA(r)
	return core.MakeReport(core.ScenarioWorkerReject, r.coord, r.decision, r.entry, r.system), nil
}

// B：Worker-B 无响应导致 Phase-1 超时并 ABORT。
func runScenarioB(root string) (core.Report, error) {
	r, err := runCore(root, coreSpec{scenario: core.ScenarioWorkerTimeout, workerCount: 2, forceNoRespOnB: true, phase1Timeout: 220 * time.Millisecond, decisionWait: 150 * time.Millisecond})
	if err != nil {
		return core.Report{}, err
	}
	renderScenarioB(r)
	return core.MakeReport(core.ScenarioWorkerTimeout, r.coord, r.decision, r.entry, r.system), nil
}

// C：Coordinator 在发送 VOTE-REQ 前崩溃，Worker INIT 超时自 ABORT。
func runScenarioC(root string) (core.Report, error) {
	r, err := runCore(root, coreSpec{scenario: core.ScenarioCoordCrashPhase1, workerCount: 2, crashBeforeVote: true, initTimeout: 250 * time.Millisecond})
	if err != nil {
		return core.Report{}, err
	}
	renderScenarioC(r)
	return core.MakeReport(core.ScenarioCoordCrashPhase1, r.coord, r.decision, r.entry, r.system), nil
}

// D：Coordinator 在 DECISION 落盘后、广播前崩溃；可选恢复重放。
func runScenarioD(root string) (core.Report, error) {
	r, err := runCore(root, coreSpec{scenario: core.ScenarioCoordCrashPhase2, workerCount: 2, crashAfterDecision: true, recoverAfterCrash: true, phase1Timeout: 250 * time.Millisecond, decisionWait: 300 * time.Millisecond})
	if err != nil {
		return core.Report{}, err
	}
	renderScenarioD(r)
	return core.MakeReport(core.ScenarioCoordCrashPhase2, r.coord, r.decision, r.entry, r.system), nil
}

// renderScenarioNormal 根据核心结果动态渲染 normal 剧情。
func renderScenarioNormal(r coreResult) {
	lines := []utils.CinematicLine{
		{Speaker: "教授", Text: "各组汇报状态。没有我的最终指令，任何人不得行动。"},
		{Speaker: "东京/丹佛", Text: "入口已就位，准备完成。我们可提交（VOTE-COMMIT）。"},
		{Speaker: "里约", Text: "系统已切换，准备完成。可提交（VOTE-COMMIT）。"},
		{Speaker: "柏林/内罗毕", Text: "印刷链路与油墨全部就绪。可提交（VOTE-COMMIT）。"},
	}
	if r.decision == core.StateCOMMIT {
		lines = append(lines,
			utils.CinematicLine{Speaker: "旁白", Text: "教授收齐全部 YES，写入决议 DECISION=COMMIT。"},
			utils.CinematicLine{Speaker: "教授", Text: "所有人确认完毕，let's ↘ go↗~（GLOBAL-COMMIT）"},
		)
	} else {
		lines = append(lines,
			utils.CinematicLine{Speaker: "旁白", Text: "本轮未收齐有效 YES，教授改为保守决议。"},
			utils.CinematicLine{Speaker: "教授", Text: "GLOBAL-ABORT。所有人回到待命，不做局部推进。"},
		)
	}
	lines = append(lines, utils.CinematicLine{Speaker: "旁白", Text: buildStateSummaryLine(r)})
	utils.RenderCinematicScene("正常场景：教授的完美协同实验（2PC 成功提交）", []string{"Aurora 印钞厂内，教授准备验证“零冲突同步协作”理论。", "所有小组先确认可执行，再等待统一指令。"}, lines)
}

// renderScenarioA 根据决议分支动态渲染“拒票”剧情。
func renderScenarioA(r coreResult) {
	lines := []utils.CinematicLine{{Speaker: "教授", Text: "第二轮任务：封装后的纸钞，3 分钟内转运到地下金库。各组给投票。"}, {Speaker: "东京/丹佛", Text: "入口走廊可控，运钞通道畅通，我们给 YES。"}}
	if r.decision == core.StateABORT {
		lines = append(lines,
			utils.CinematicLine{Speaker: "里约", Text: "我给 NO。警督已锁定两段监控盲区，转运窗口不成立。"},
			utils.CinematicLine{Speaker: "教授", Text: "拒票成立，GLOBAL-ABORT。所有人留在原位，不做局部推进。"},
		)
	} else {
		lines = append(lines, utils.CinematicLine{Speaker: "里约", Text: "条件已满足，我给 YES。"})
	}
	lines = append(lines, utils.CinematicLine{Speaker: "旁白", Text: buildStateSummaryLine(r)})
	utils.RenderCinematicScene("故障A：拒票（VOTE-ABORT）", []string{"正常场景刚成功后，教授准备发起第二轮协同动作。"}, lines)
}

// renderScenarioB 根据超时/决议分支动态渲染“无响应”剧情。
func renderScenarioB(r coreResult) {
	lines := []utils.CinematicLine{{Speaker: "教授", Text: "上一轮否决我接受，这一轮按新窗口再投一次。各组回复状态。"}, {Speaker: "东京/丹佛", Text: "入口组仍可执行，给 YES。"}}
	if hasCoreStep(r, "PHASE1_TIMEOUT") || r.decision == core.StateABORT {
		lines = append(lines,
			utils.CinematicLine{Speaker: "里约", Text: "...（链路抖动）... 无法稳定回传 ..."},
			utils.CinematicLine{Speaker: "教授", Text: "未收齐投票，GLOBAL-ABORT。全员回滚，准备下一轮。"},
		)
	}
	lines = append(lines, utils.CinematicLine{Speaker: "旁白", Text: buildStateSummaryLine(r)})
	utils.RenderCinematicScene("故障B：超时无响应（TIMEOUT）", []string{"2PC 规则要求：在 WAIT 阶段超时未收齐投票，必须全局回滚。"}, lines)
}

// renderScenarioC 根据是否触发崩溃分支渲染“一阶段崩溃”剧情。
func renderScenarioC(r coreResult) {
	lines := []utils.CinematicLine{{Speaker: "教授", Text: "第三轮开始前最后确认：我将发起 VOTE-REQ。"}}
	if hasCoreStep(r, "COORD_CRASH_BEFORE_VOTE") {
		lines = append(lines,
			utils.CinematicLine{Speaker: "旁白", Text: "主控链路瞬断，教授在发包前离线。"},
			utils.CinematicLine{Speaker: "里约", Text: "系统组同样 INIT；超时计时器已启动。"},
		)
	}
	lines = append(lines, utils.CinematicLine{Speaker: "旁白", Text: buildStateSummaryLine(r)})
	utils.RenderCinematicScene("故障C：第一阶段崩溃（Coordinator Crash Before Vote）", []string{"若参与者长期停在 INIT 且未收到投票请求，只能超时自 ABORT。"}, lines)
}

// renderScenarioD 根据是否发生恢复重放动态渲染 D 场景。
func renderScenarioD(r coreResult) {
	lines := []utils.CinematicLine{{Speaker: "教授", Text: "收齐 YES，先落盘 DECISION=COMMIT，再统一广播。"}}
	hasCrash := hasCoreStep(r, "COORD_CRASH_AFTER_DECISION")
	hasReplay := hasCoreStep(r, "RECOVER_REPLAY_DONE")
	if hasCrash {
		lines = append(lines,
			utils.CinematicLine{Speaker: "旁白", Text: "决议刚写入，广播尚未发出，主控信号再次中断。"},
			utils.CinematicLine{Speaker: "东京/丹佛", Text: "入口组停在 READY，按协议等待，不擅自推进。"},
		)
	}
	if hasReplay {
		lines = append(lines,
			utils.CinematicLine{Speaker: "旁白", Text: "教授恢复上线，读取稳定日志，确认旧决议是 COMMIT。"},
			utils.CinematicLine{Speaker: "教授", Text: "按日志重放：GLOBAL-COMMIT。所有组继续执行。"},
		)
	} else if hasCrash {
		lines = append(lines,
			utils.CinematicLine{Speaker: "警督", Text: "这次你们会被卡死在中间态。"},
			utils.CinematicLine{Speaker: "旁白", Text: "当前演示停在“已写决议但未重放”状态：Worker 继续阻塞等待。"},
		)
	}
	lines = append(lines, utils.CinematicLine{Speaker: "旁白", Text: buildStateSummaryLine(r)})
	utils.RenderCinematicScene("故障D：决议已写盘，但广播前失联", []string{"READY 节点必须等待最终决议，任何人不得擅自提交或回滚。"}, lines)
}
