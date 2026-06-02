package core

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type PlanState string

const (
	StateINIT   PlanState = "INIT"
	StateWAIT   PlanState = "WAIT"
	StateREADY  PlanState = "READY"
	StateCOMMIT PlanState = "COMMIT"
	StateABORT  PlanState = "ABORT"
)

type Scenario string

const (
	ScenarioNormal           Scenario = "normal"
	ScenarioWorkerReject     Scenario = "a"
	ScenarioWorkerTimeout    Scenario = "b"
	ScenarioCoordCrashPhase1 Scenario = "c"
	ScenarioCoordCrashPhase2 Scenario = "d"
)

// StoryPlanID / StoryOperation 是教学剧情里统一使用的计划标识与操作名。
const (
	StoryPlanID    = "plan-1001"
	StoryOperation = "aurora_heist_sync"
)

// Report 是场景执行结束后对外暴露的简要结果。
type Report struct {
	Scenario         Scenario
	CoordinatorState PlanState
	Decision         PlanState
	WorkerStates     map[string]PlanState
}

type logRecord struct {
	Time    string                 `json:"time"`
	Node    string                 `json:"node"`
	Kind    string                 `json:"kind"`
	Payload map[string]interface{} `json:"payload"`
}

type StableLogger struct {
	node string
	file string
	mu   sync.Mutex
}

// NewStableLogger 创建稳定日志对象（线程安全）。
func NewStableLogger(node, file string) *StableLogger {
	return &StableLogger{node: node, file: file}
}

// Persist 将关键事件以 JSON 行方式落盘，支持恢复与回放教学。
func (l *StableLogger) Persist(kind string, payload map[string]interface{}) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.file), 0o755); err != nil {
		return err
	}
	rec := logRecord{
		Time:    time.Now().Format(time.RFC3339Nano),
		Node:    l.node,
		Kind:    kind,
		Payload: payload,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}

	return nil
}

func transitionAllowed(from, to PlanState) bool {
	switch from {
	case StateINIT:
		return to == StateWAIT || to == StateREADY || to == StateABORT
	case StateWAIT:
		return to == StateCOMMIT || to == StateABORT
	case StateREADY:
		return to == StateCOMMIT || to == StateABORT
	default:
		return false
	}
}

// TransitionState 负责状态迁移检查 + BEFORE/AFTER 双日志落盘。
func TransitionState(node string, logger *StableLogger, current *PlanState, to PlanState, reason string) error {
	from := *current
	if from == to {
		return nil
	}
	if !transitionAllowed(from, to) {
		return fmt.Errorf("非法状态迁移: %s -> %s", from, to)
	}

	before := map[string]interface{}{"from": from, "to": to, "reason": reason}
	if err := logger.Persist("STATE_BEFORE", before); err != nil {
		return err
	}
	*current = to
	after := map[string]interface{}{"state": to, "reason": reason}
	return logger.Persist("STATE_AFTER", after)
}

// Worker 是 2PC 参与者。
type Worker struct {
	ID            string
	State         PlanState
	DecisionCh    chan PlanState
	Logger        *StableLogger
	ForceReject   bool
	ForceNoResp   bool
	NoRespReason  string
	InitStartedAt time.Time
}

var ErrWorkerNoResponse = errors.New("worker no response")

// NewWorker 创建参与者并初始化为 INIT。
func NewWorker(id string, logger *StableLogger) *Worker {
	return &Worker{
		ID:         id,
		State:      StateINIT,
		DecisionCh: make(chan PlanState, 1),
		Logger:     logger,
	}
}

// BeginPlan 初始化计划执行上下文并写入 PLAN_BEGIN。
func (w *Worker) BeginPlan(planID string) error {
	w.State = StateINIT
	w.InitStartedAt = time.Now()
	return w.Logger.Persist("PLAN_BEGIN", map[string]interface{}{"planID": planID, "state": w.State, "operation": StoryOperation})
}

// OnVoteReq 处理第一阶段投票请求（YES / NO / SILENT）。
func (w *Worker) OnVoteReq(planID string) (bool, error) {
	if w.ForceNoResp {
		reason := w.NoRespReason
		if reason == "" {
			reason = "链路故障/干扰，未返回投票"
		}
		_ = w.Logger.Persist("VOTE_SILENT", map[string]interface{}{"planID": planID, "reason": reason})
		return false, ErrWorkerNoResponse
	}

	if w.ForceReject {
		if err := TransitionState(w.ID, w.Logger, &w.State, StateABORT, "投票拒绝：执行窗口不成立（监控/通道/条件异常）"); err != nil {
			return false, err
		}
		return false, nil
	}

	if err := TransitionState(w.ID, w.Logger, &w.State, StateREADY, "投票同意，进入 READY 等待全局决议"); err != nil {
		return false, err
	}
	return true, nil
}

// AwaitDecisionBlocking 在 READY 状态等待第二阶段全局决议。
func (w *Worker) AwaitDecisionBlocking(wait time.Duration) PlanState {
	if w.State != StateREADY {
		return w.State
	}

	select {
	case decision := <-w.DecisionCh:
		if decision == StateCOMMIT {
			_ = TransitionState(w.ID, w.Logger, &w.State, StateCOMMIT, "收到 GLOBAL-COMMIT")
		} else {
			_ = TransitionState(w.ID, w.Logger, &w.State, StateABORT, "收到 GLOBAL-ABORT")
		}
	case <-time.After(wait):
	}
	return w.State
}

// AbortIfInitTimeout 用于 C 场景：INIT 长时间未收请求则自 ABORT。
func (w *Worker) AbortIfInitTimeout(timeout time.Duration, planID string) {
	time.Sleep(timeout)
	if w.State == StateINIT {
		_ = TransitionState(w.ID, w.Logger, &w.State, StateABORT, "INIT 等待 VOTE-REQ 超时，自动 ABORT")
		_ = w.Logger.Persist("TIMEOUT_ABORT", map[string]interface{}{"planID": planID})
	}
}

// DeliverGlobalDecision 将 GLOBAL-* 决议投递给参与者。
func (w *Worker) DeliverGlobalDecision(decision PlanState) {
	select {
	case w.DecisionCh <- decision:
	default:
	}
}

// Coordinator 是 2PC 协调者。
type Coordinator struct {
	State   PlanState
	Logger  *StableLogger
	Workers []*Worker
}

// NewCoordinator 创建协调者。
func NewCoordinator(logger *StableLogger, ws []*Worker) *Coordinator {
	return &Coordinator{State: StateINIT, Logger: logger, Workers: ws}
}

// BroadcastDecision 向所有参与者广播全局决议。
func (c *Coordinator) BroadcastDecision(decision PlanState) {
	for _, w := range c.Workers {
		w.DeliverGlobalDecision(decision)
	}
}

// LoadPersistedDecision 从协调者日志恢复最近一次 DECISION。
func (c *Coordinator) LoadPersistedDecision() (PlanState, error) {
	f, err := os.Open(c.Logger.file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var decision PlanState
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		var rec logRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.Kind == "DECISION" {
			if v, ok := rec.Payload["decision"].(string); ok {
				decision = PlanState(v)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if decision == "" {
		return "", errors.New("no persisted decision found")
	}
	return decision, nil
}

// RecoverAndReplay 用于 D 场景：恢复后重放 GLOBAL-*。
func (c *Coordinator) RecoverAndReplay() (PlanState, error) {
	decision, err := c.LoadPersistedDecision()
	if err != nil {
		return "", err
	}
	if err := c.Logger.Persist("RECOVER_LOAD", map[string]interface{}{"decision": decision}); err != nil {
		return "", err
	}
	c.State = StateWAIT
	c.BroadcastDecision(decision)
	if decision == StateCOMMIT {
		if err := TransitionState("Coordinator-Recover", c.Logger, &c.State, StateCOMMIT, "恢复后重放 GLOBAL-COMMIT"); err != nil {
			return "", err
		}
	} else {
		if err := TransitionState("Coordinator-Recover", c.Logger, &c.State, StateABORT, "恢复后重放 GLOBAL-ABORT"); err != nil {
			return "", err
		}
	}
	return decision, nil
}

// MakeReport 汇总对外报告。
func MakeReport(s Scenario, c *Coordinator, decision PlanState, ws ...*Worker) Report {
	m := make(map[string]PlanState, len(ws))
	for _, w := range ws {
		m[w.ID] = w.State
	}
	return Report{
		Scenario:         s,
		CoordinatorState: c.State,
		Decision:         decision,
		WorkerStates:     m,
	}
}
