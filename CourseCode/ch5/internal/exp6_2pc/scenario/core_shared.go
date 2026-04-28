package scenario

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ch5/internal/exp6_2pc/core"
)

type coreStep struct {
	// Label 是该快照对应的阶段标签（如 PHASE1_WAIT / RECOVER_REPLAY_DONE）。
	Label string
	// CState / AState / BState / C2State 分别记录协调者与参与者在该阶段的状态。
	CState  core.PlanState
	AState  core.PlanState
	BState  core.PlanState
	C2State core.PlanState
}

type coreResult struct {
	// scenario 标识本次执行属于哪个场景（normal / a / b / c / d）。
	scenario core.Scenario
	// 下面是本次执行关联的节点对象引用，供渲染与报告汇总使用。
	coord    *core.Coordinator
	entry    *core.Worker
	system   *core.Worker
	printing *core.Worker
	// decision 是协调者在第一阶段结束后写入稳定存储的全局决议。
	decision core.PlanState
	// steps 按时序记录每个关键阶段的状态快照，供动态渲染分支使用。
	steps []coreStep
}

type coreSpec struct {
	// 基础场景配置
	scenario    core.Scenario
	workerCount int
	// Phase-1 故障注入：拒票 / 无响应
	forceRejectOnB bool
	forceNoRespOnB bool
	// 协调者崩溃注入点（Phase-1 前 / Phase-2 广播前）
	crashBeforeVote    bool
	crashAfterDecision bool
	// D 场景专用：是否在崩溃后执行恢复重放
	recoverAfterCrash bool
	// 超时配置
	phase1Timeout time.Duration
	decisionWait  time.Duration
	initTimeout   time.Duration
}

// normalizeCoreSpec 做参数归一化，避免在执行主流程里混杂大量默认值判断。
func normalizeCoreSpec(spec coreSpec) coreSpec {
	if spec.workerCount <= 0 {
		spec.workerCount = 2
	}
	if spec.phase1Timeout <= 0 {
		spec.phase1Timeout = 250 * time.Millisecond
	}
	if spec.decisionWait <= 0 {
		spec.decisionWait = 150 * time.Millisecond
	}
	if spec.initTimeout <= 0 {
		spec.initTimeout = 250 * time.Millisecond
	}
	return spec
}

func runCore(root string, spec coreSpec) (coreResult, error) {
	// ========================================================
	// 0) 配置归一化阶段
	// ========================================================
	spec = normalizeCoreSpec(spec)

	// ========================================================
	// 1) 环境初始化阶段
	//    - 清理并创建场景目录
	//    - 构造 Worker/Coordinator
	//    - 所有 Worker 进入 INIT（PLAN_BEGIN）
	// ========================================================
	scenarioDir := filepath.Join(root, string(spec.scenario))
	if err := os.RemoveAll(scenarioDir); err != nil {
		return coreResult{}, err
	}
	if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
		return coreResult{}, err
	}

	entry := core.NewWorker("Worker-A", core.NewStableLogger("Worker-A", filepath.Join(scenarioDir, "worker_a.log")))
	system := core.NewWorker("Worker-B", core.NewStableLogger("Worker-B", filepath.Join(scenarioDir, "worker_b.log")))
	var printing *core.Worker
	if spec.workerCount >= 3 {
		printing = core.NewWorker("Worker-C", core.NewStableLogger("Worker-C", filepath.Join(scenarioDir, "worker_c.log")))
	}

	workers := []*core.Worker{entry, system}
	if printing != nil {
		workers = append(workers, printing)
	}

	professor := core.NewCoordinator(core.NewStableLogger("Coordinator", filepath.Join(scenarioDir, "coordinator.log")), workers)

	for _, w := range workers {
		if err := w.BeginPlan(core.StoryPlanID); err != nil {
			return coreResult{}, err
		}
	}

	system.ForceReject = spec.forceRejectOnB
	system.ForceNoResp = spec.forceNoRespOnB

	res := coreResult{
		scenario: spec.scenario,
		coord:    professor,
		entry:    entry,
		system:   system,
		printing: printing,
		decision: core.StateABORT,
		steps:    []coreStep{},
	}
	res.appendStep("INIT")

	if err := professor.Logger.Persist("START_PLAN", map[string]interface{}{"planID": core.StoryPlanID, "operation": core.StoryOperation}); err != nil {
		return coreResult{}, err
	}

	// ========================================================
	// 2) C 场景分支：Phase-1 发起前协调者崩溃
	//    Worker 在 INIT 超时后自动 ABORT
	// ========================================================
	if spec.crashBeforeVote {
		if err := professor.Logger.Persist("CRASH", map[string]interface{}{"phase": "before_vote_req", "planID": core.StoryPlanID}); err != nil {
			return coreResult{}, err
		}
		res.appendStep("COORD_CRASH_BEFORE_VOTE")
		for _, w := range workers {
			go w.AbortIfInitTimeout(spec.initTimeout, core.StoryPlanID)
		}
		time.Sleep(spec.initTimeout + 100*time.Millisecond)
		res.decision = core.StateABORT
		res.appendStep("INIT_TIMEOUT_ABORT")
		return res, nil
	}

	// ========================================================
	// 3) 2PC 第一阶段（Phase 1: Prepare / Vote）
	//    3.1 Coordinator: INIT -> WAIT，发送 VOTE-REQ
	//    3.2 Workers 并发执行本地检查并投票
	//    3.3 Coordinator 收票：收齐即结束；超时则进入 ABORT 路径
	//    3.4 将全局决议（DECISION）写入稳定存储
	// ========================================================
	if err := core.TransitionState("Coordinator", professor.Logger, &professor.State, core.StateWAIT, "发送 VOTE-REQ，等待全部投票"); err != nil {
		return coreResult{}, err
	}
	res.appendStep("PHASE1_WAIT")

	// voteResult 是参与者投票结果的汇总结构。
	type voteResult struct {
		ok  bool
		err error
	}
	// 并发收集每个参与者投票结果。
	voteCh := make(chan voteResult, len(workers))
	for _, w := range workers {
		go func(ww *core.Worker) {
			ok, err := ww.OnVoteReq(core.StoryPlanID)
			voteCh <- voteResult{ok: ok, err: err}
		}(w)
	}

	deadline := time.After(spec.phase1Timeout) // 第一阶段等待截止时间
	received := 0
	allCommit := true
	phase1Timeout := false
	for received < len(workers) {
		select {
		case v := <-voteCh:
			if errors.Is(v.err, core.ErrWorkerNoResponse) {
				continue
			}
			if v.err != nil {
				return coreResult{}, v.err
			}
			received++
			if !v.ok {
				allCommit = false
			}
		case <-deadline:
			phase1Timeout = true
			allCommit = false
			received = len(workers)
		}
	}
	res.appendStep("PHASE1_VOTE_DONE")

	if phase1Timeout {
		if err := professor.Logger.Persist("PHASE1_TIMEOUT", map[string]interface{}{"planID": core.StoryPlanID, "reason": "WAIT 超时，未收齐全部投票"}); err != nil {
			return coreResult{}, err
		}
	}

	decision := core.StateABORT
	if allCommit {
		decision = core.StateCOMMIT
	}
	res.decision = decision

	if err := professor.Logger.Persist("DECISION", map[string]interface{}{"planID": core.StoryPlanID, "decision": decision}); err != nil {
		return coreResult{}, err
	}
	res.appendStep("PHASE1_DECISION_PERSISTED")

	// ========================================================
	// 4) D 场景分支：决议已写盘，但 Phase-2 广播前崩溃
	//    - Worker 停在 READY 阻塞等待
	//    - 可选恢复：读取持久化决议并重放 GLOBAL-*
	// ========================================================
	if spec.crashAfterDecision {
		if err := professor.Logger.Persist("CRASH", map[string]interface{}{"phase": "after_decision_before_broadcast", "planID": core.StoryPlanID, "decision": decision}); err != nil {
			return coreResult{}, err
		}
		res.appendStep("COORD_CRASH_AFTER_DECISION")

		awaitWorkersDecision(workers, 220*time.Millisecond)
		res.appendStep("WORKERS_BLOCKING_READY")

		if spec.recoverAfterCrash {
			recoverCoord := core.NewCoordinator(professor.Logger, workers)
			// 读取 DECISION 并重放，推动 READY 节点完成终态收敛。
			recoveredDecision, err := recoverCoord.RecoverAndReplay()
			if err != nil {
				return coreResult{}, err
			}
			res.coord = recoverCoord
			res.decision = recoveredDecision
			awaitWorkersDecision(workers, 300*time.Millisecond)
			res.appendStep("RECOVER_REPLAY_DONE")
		}
		return res, nil
	}

	// ========================================================
	// 5) 2PC 第二阶段（Phase 2: Commit / Abort）
	//    5.1 Coordinator 广播 GLOBAL-COMMIT / GLOBAL-ABORT
	//    5.2 Coordinator 落地终态
	//    5.3 Workers 并发接收决议并收敛
	// ========================================================
	professor.BroadcastDecision(decision)
	if decision == core.StateCOMMIT {
		if err := core.TransitionState("Coordinator", professor.Logger, &professor.State, core.StateCOMMIT, "广播 GLOBAL-COMMIT"); err != nil {
			return coreResult{}, err
		}
	} else {
		reason := "收到拒票，广播 GLOBAL-ABORT"
		if phase1Timeout {
			reason = "WAIT 超时，广播 GLOBAL-ABORT"
		}
		if err := core.TransitionState("Coordinator", professor.Logger, &professor.State, core.StateABORT, reason); err != nil {
			return coreResult{}, err
		}
	}
	res.appendStep("PHASE2_BROADCAST_DONE")
	awaitWorkersDecision(workers, spec.decisionWait)
	res.appendStep("PHASE2_WORKERS_DONE")

	return res, nil
}

// awaitWorkersDecision 并发等待所有 Worker 完成决议收敛。
func awaitWorkersDecision(workers []*core.Worker, wait time.Duration) {
	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(ww *core.Worker) {
			defer wg.Done()
			ww.AwaitDecisionBlocking(wait)
		}(w)
	}
	wg.Wait()
}

// appendStep 记录一个阶段快照，作为渲染层动态判断依据。
func (r *coreResult) appendStep(label string) {
	r.steps = append(r.steps, coreStep{
		Label:   label,
		CState:  r.coord.State,
		AState:  r.entry.State,
		BState:  r.system.State,
		C2State: pickState(r.printing),
	})
}

// pickState 安全读取可选 Worker 的状态（normal 有 Worker-C，其余场景可能没有）。
func pickState(w *core.Worker) core.PlanState {
	if w == nil {
		return ""
	}
	return w.State
}

// hasCoreStep 判断是否走过某个关键阶段标签。
func hasCoreStep(r coreResult, label string) bool {
	for _, s := range r.steps {
		if s.Label == label {
			return true
		}
	}
	return false
}

// buildStateSummaryLine 生成统一状态收束文案，供剧情最后一行使用。
func buildStateSummaryLine(r coreResult) string {
	if len(r.steps) == 0 {
		return "状态收束：无可用步骤快照。"
	}
	last := r.steps[len(r.steps)-1]
	if r.printing != nil {
		return fmt.Sprintf("状态收束：Coordinator=%s，Worker-A=%s，Worker-B=%s，Worker-C=%s。", last.CState, last.AState, last.BState, last.C2State)
	}
	return fmt.Sprintf("状态收束：Coordinator=%s，Worker-A=%s，Worker-B=%s。", last.CState, last.AState, last.BState)
}
