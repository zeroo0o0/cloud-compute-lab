package exp6_2pc

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

// TxState 是 2PC 状态机的统一状态定义。
//
// 协调者与参与者都使用该状态集合，方便在日志、报告、剧情渲染中统一表达。
type TxState string

const (
	StateINIT   TxState = "INIT"
	StateWAIT   TxState = "WAIT"
	StateREADY  TxState = "READY"
	StateCOMMIT TxState = "COMMIT"
	StateABORT  TxState = "ABORT"
)

// Scenario 定义剧情演绎模式下可执行的场景编号。
type Scenario string

const (
	ScenarioNormal           Scenario = "normal"
	ScenarioWorkerReject     Scenario = "a"
	ScenarioWorkerTimeout    Scenario = "b"
	ScenarioCoordCrashPhase1 Scenario = "c"
	ScenarioCoordCrashPhase2 Scenario = "d"
)

// visualStepDelay 控制电影式对白输出节奏（毫秒级）。
var visualStepDelay = 650 * time.Millisecond

// SetVisualStepDelay 设置剧情对白的推进速度。
func SetVisualStepDelay(ms int) {
	if ms <= 0 {
		visualStepDelay = 0
		return
	}
	visualStepDelay = time.Duration(ms) * time.Millisecond
}

// Report 是每个场景结束后对外返回的摘要。
type Report struct {
	Scenario         Scenario
	CoordinatorState TxState
	Decision         TxState
	WorkerStates     map[string]TxState
}

// logRecord 是稳定存储日志的 JSON 行结构。
type logRecord struct {
	Time    string                 `json:"time"`
	Node    string                 `json:"node"`
	Kind    string                 `json:"kind"`
	Payload map[string]interface{} `json:"payload"`
}

// stableLogger 负责将关键事件落盘，用于恢复与教学回放。
type stableLogger struct {
	node string
	file string
	mu   sync.Mutex
}

func newStableLogger(node, file string) *stableLogger {
	return &stableLogger{node: node, file: file}
}

func (l *stableLogger) persist(kind string, payload map[string]interface{}) error {
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

// transitionAllowed 定义有限状态机允许的迁移边。
func transitionAllowed(from, to TxState) bool {
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

// transitionState 在状态变更前后写盘，保证可追溯与可恢复。
func transitionState(node string, logger *stableLogger, current *TxState, to TxState, reason string) error {
	from := *current
	if from == to {
		return nil
	}
	if !transitionAllowed(from, to) {
		return fmt.Errorf("非法状态迁移: %s -> %s", from, to)
	}

	before := map[string]interface{}{"from": from, "to": to, "reason": reason}
	if err := logger.persist("STATE_BEFORE", before); err != nil {
		return err
	}
	*current = to
	after := map[string]interface{}{"state": to, "reason": reason}
	return logger.persist("STATE_AFTER", after)
}

// worker 表示 2PC 参与者，持有自身状态与接收全局决议的通道。
type worker struct {
	id            string
	balance       int
	state         TxState
	decisionCh    chan TxState
	logger        *stableLogger
	forceReject   bool
	forceNoResp   bool
	initStartedAt time.Time
}

func newWorker(id string, balance int, logger *stableLogger) *worker {
	return &worker{
		id:         id,
		balance:    balance,
		state:      StateINIT,
		decisionCh: make(chan TxState, 1),
		logger:     logger,
	}
}

func (w *worker) beginTx(txID string) error {
	w.state = StateINIT
	w.initStartedAt = time.Now()
	return w.logger.persist("TX_BEGIN", map[string]interface{}{"txID": txID, "state": w.state, "balance": w.balance})
}

// onVoteReq 实现参与者在第一阶段的投票逻辑。
func (w *worker) onVoteReq(txID string, amount int) (bool, error) {
	if w.forceNoResp {
		return false, errors.New("worker no response")
	}

	if w.forceReject || w.balance < amount {
		if err := transitionState(w.id, w.logger, &w.state, StateABORT, "投票拒绝：余额不足或故障注入"); err != nil {
			return false, err
		}
		return false, nil
	}

	if err := transitionState(w.id, w.logger, &w.state, StateREADY, "投票同意，进入 READY 等待全局决议"); err != nil {
		return false, err
	}
	return true, nil
}

// awaitDecisionBlocking 在 READY 状态阻塞等待 GLOBAL-* 决议。
func (w *worker) awaitDecisionBlocking(wait time.Duration) TxState {
	if w.state != StateREADY {
		return w.state
	}

	select {
	case decision := <-w.decisionCh:
		if decision == StateCOMMIT {
			_ = transitionState(w.id, w.logger, &w.state, StateCOMMIT, "收到 GLOBAL-COMMIT")
		} else {
			_ = transitionState(w.id, w.logger, &w.state, StateABORT, "收到 GLOBAL-ABORT")
		}
	case <-time.After(wait):
	}
	return w.state
}

func (w *worker) abortIfInitTimeout(timeout time.Duration, txID string) {
	time.Sleep(timeout)
	if w.state == StateINIT {
		_ = transitionState(w.id, w.logger, &w.state, StateABORT, "INIT 等待 VOTE-REQ 超时，自动 ABORT")
		_ = w.logger.persist("TIMEOUT_ABORT", map[string]interface{}{"txID": txID})
	}
}

func (w *worker) deliverGlobalDecision(decision TxState) {
	select {
	case w.decisionCh <- decision:
	default:
	}
}

// coordinator 表示 2PC 协调者，负责发起投票、决议并广播。
type coordinator struct {
	state  TxState
	logger *stableLogger
	ws     []*worker
}

func newCoordinator(logger *stableLogger, ws []*worker) *coordinator {
	return &coordinator{state: StateINIT, logger: logger, ws: ws}
}

// runOptions 控制每个场景执行时的故障注入和超时参数。
type runOptions struct {
	phase1Timeout        time.Duration
	crashBeforeVoteReq   bool
	crashAfterDecision   bool
	decisionWaitDeadline time.Duration
	scenario             Scenario
}

const (
	storyTxID        = "tx-1001"
	storyTradeAmount = 30
)

func stateBadge(s TxState) string {
	switch s {
	case StateINIT:
		return "INIT"
	case StateWAIT:
		return "WAIT"
	case StateREADY:
		return "READY"
	case StateCOMMIT:
		return "COMMIT"
	case StateABORT:
		return "ABORT"
	default:
		return string(s)
	}
}

// runTx 是 2PC 的主执行流程：投票请求 -> 收集投票 -> 写决议 -> 广播决议。
func (c *coordinator) runTx(txID string, amount int, opt runOptions) (TxState, error) {
	if err := c.logger.persist("START_TX", map[string]interface{}{"txID": txID, "amount": amount}); err != nil {
		return "", err
	}

	if opt.crashBeforeVoteReq {
		if err := c.logger.persist("CRASH", map[string]interface{}{"phase": "before_vote_req", "txID": txID}); err != nil {
			return "", err
		}
		return "", nil
	}

	if err := transitionState("Coordinator", c.logger, &c.state, StateWAIT, "发送 VOTE-REQ，等待所有 Worker 投票"); err != nil {
		return "", err
	}

	type vote struct {
		workerID string
		ok       bool
		err      error
	}
	voteCh := make(chan vote, len(c.ws))
	for _, w := range c.ws {
		go func(ww *worker) {
			ok, err := ww.onVoteReq(txID, amount)
			voteCh <- vote{workerID: ww.id, ok: ok, err: err}
		}(w)
	}

	deadline := time.After(opt.phase1Timeout)
	received := 0
	allCommit := true
	for received < len(c.ws) {
		select {
		case v := <-voteCh:
			received++
			if v.err != nil || !v.ok {
				allCommit = false
			}
		case <-deadline:
			allCommit = false
			received = len(c.ws)
		}
	}

	decision := StateABORT
	if allCommit {
		decision = StateCOMMIT
	}

	if err := c.logger.persist("DECISION", map[string]interface{}{"txID": txID, "decision": decision}); err != nil {
		return "", err
	}

	if opt.crashAfterDecision {
		if err := c.logger.persist("CRASH", map[string]interface{}{"phase": "after_decision_before_broadcast", "txID": txID, "decision": decision}); err != nil {
			return "", err
		}
		return decision, nil
	}

	c.broadcastDecision(decision)
	if decision == StateCOMMIT {
		if err := transitionState("Coordinator", c.logger, &c.state, StateCOMMIT, "广播 GLOBAL-COMMIT 完成"); err != nil {
			return decision, err
		}
	} else {
		if err := transitionState("Coordinator", c.logger, &c.state, StateABORT, "广播 GLOBAL-ABORT 完成"); err != nil {
			return decision, err
		}
	}

	for _, w := range c.ws {
		w.awaitDecisionBlocking(opt.decisionWaitDeadline)
	}

	return decision, nil
}

func (c *coordinator) broadcastDecision(decision TxState) {
	for _, w := range c.ws {
		w.deliverGlobalDecision(decision)
	}
}

// loadPersistedDecision 从协调者日志中恢复最近一次决议。
func (c *coordinator) loadPersistedDecision() (TxState, error) {
	f, err := os.Open(c.logger.file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var decision TxState
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		var rec logRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.Kind == "DECISION" {
			if v, ok := rec.Payload["decision"].(string); ok {
				decision = TxState(v)
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

// recoverAndReplay 用于 D 场景：重启后读取决议并重放 GLOBAL-* 广播。
func (c *coordinator) recoverAndReplay() (TxState, error) {
	decision, err := c.loadPersistedDecision()
	if err != nil {
		return "", err
	}
	if err := c.logger.persist("RECOVER_LOAD", map[string]interface{}{"decision": decision}); err != nil {
		return "", err
	}
	c.state = StateWAIT
	c.broadcastDecision(decision)
	if decision == StateCOMMIT {
		if err := transitionState("Coordinator-Recover", c.logger, &c.state, StateCOMMIT, "恢复后重放 GLOBAL-COMMIT"); err != nil {
			return "", err
		}
	} else {
		if err := transitionState("Coordinator-Recover", c.logger, &c.state, StateABORT, "恢复后重放 GLOBAL-ABORT"); err != nil {
			return "", err
		}
	}
	return decision, nil
}

// RunScenario 是对外统一入口：根据场景路由到对应的 core+dialogue 执行器。
func RunScenario(s Scenario, rootDataDir string) (Report, error) {
	switch s {
	case ScenarioNormal:
		return runScenarioNormal(rootDataDir)
	case ScenarioWorkerReject:
		return runScenarioA(rootDataDir)
	case ScenarioWorkerTimeout:
		return runScenarioB(rootDataDir)
	case ScenarioCoordCrashPhase1:
		return runScenarioC(rootDataDir)
	case ScenarioCoordCrashPhase2:
		return runScenarioD(rootDataDir)
	default:
		return Report{}, fmt.Errorf("unknown scenario: %s", s)
	}
}

// setup 为单个场景初始化隔离的数据目录、节点与初始事务日志。
func setup(name Scenario, rootDataDir string) (*coordinator, *worker, *worker, error) {
	scenarioDir := filepath.Join(rootDataDir, string(name))
	if err := os.RemoveAll(scenarioDir); err != nil {
		return nil, nil, nil, err
	}
	if err := os.MkdirAll(scenarioDir, 0o755); err != nil {
		return nil, nil, nil, err
	}

	wA := newWorker("Worker-A", 100, newStableLogger("Worker-A", filepath.Join(scenarioDir, "worker_a.log")))
	wB := newWorker("Worker-B", 100, newStableLogger("Worker-B", filepath.Join(scenarioDir, "worker_b.log")))
	coord := newCoordinator(newStableLogger("Coordinator", filepath.Join(scenarioDir, "coordinator.log")), []*worker{wA, wB})

	if err := wA.beginTx(storyTxID); err != nil {
		return nil, nil, nil, err
	}
	if err := wB.beginTx(storyTxID); err != nil {
		return nil, nil, nil, err
	}
	return coord, wA, wB, nil
}

// makeReport 汇总最终状态，供命令行与测试消费。
func makeReport(s Scenario, c *coordinator, decision TxState, ws ...*worker) Report {
	m := make(map[string]TxState, len(ws))
	for _, w := range ws {
		m[w.id] = w.state
	}
	return Report{
		Scenario:         s,
		CoordinatorState: c.state,
		Decision:         decision,
		WorkerStates:     m,
	}
}
