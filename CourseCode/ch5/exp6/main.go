package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type actorView struct {
	Name  string
	Role  string
	State string
	Note  string
}

type step struct {
	Event   string
	Coord   actorView
	Workers []actorView
}

type scenarioSpec struct {
	Key   string
	Title string
	Intro []string
	Rule  scenarioRule
}

type scenarioRule struct {
	RejectWorker          string
	TimeoutWorker         string
	CoordCrashBeforeVote  bool
	CoordCrashAfterCommit bool
}

type voteBehavior string

const (
	stateInit   = "INIT"
	stateWait   = "WAIT"
	stateReady  = "READY"
	stateCommit = "COMMIT"
	stateAbort  = "ABORT"
	stateDown   = "DOWN"

	roleCoord  = "Service"
	roleWorker = "Worker"

	voteYes     voteBehavior = "YES"
	voteNo      voteBehavior = "NO"
	voteTimeout voteBehavior = "TIMEOUT"
)

type voteRequest struct {
	Reply chan voteResponse
}

type voteResponse struct {
	Worker string
	Vote   voteBehavior
}

type decisionMsg struct {
	Decision string
	Ack      chan struct{}
}

type workerNode struct {
	Name       string
	Behavior   voteBehavior
	ReqCh      chan voteRequest
	DecisionCh chan decisionMsg
	Done       chan struct{}
}

type clusterState struct {
	mu      sync.Mutex
	coord   actorView
	workers []actorView
}

func main() {
	scenarioKey := flag.String("scenario", "all", "场景：normal | a | b | c | d | all")
	flag.Parse()

	specs := buildScenarioSpecs()
	if *scenarioKey == "all" {
		order := []string{"normal", "a", "b", "c", "d"}
		for i, key := range order {
			if i > 0 {
				waitForEnter("\n--- 按 Enter 继续，进入下一场景 ---")
			}
			runScenario(specs[key])
		}
		return
	}

	spec, ok := specs[*scenarioKey]
	if !ok {
		fmt.Printf("未知场景: %s\n", *scenarioKey)
		os.Exit(1)
	}
	runScenario(spec)
}

// runScenario 会启动 2PC 协调者与参与者 goroutine，通过动态事件渲染场景。
func runScenario(spec scenarioSpec) {
	fmt.Println("============================================================")
	fmt.Printf("场景：%s (%s)\n", spec.Title, spec.Key)
	for _, line := range spec.Intro {
		fmt.Println(line)
	}

	events := simulateScenario(spec)
	for i, ev := range events {
		waitForEnter(fmt.Sprintf("按 Enter 继续（步骤 %d/%d）...", i+1, len(events)))
		renderStep(spec.Title, ev, i+1, len(events))
	}
}

// simulateScenario 用多协程实现 2PC 协议，生成按步骤可视化的事件序列。
func simulateScenario(spec scenarioSpec) []step {
	state := newClusterState()
	workers := buildWorkers(spec)
	for _, worker := range workers {
		go runWorker(state, worker)
	}

	eventsCh := make(chan step, 16)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runCoordinator(state, workers, spec, eventsCh)
	}()

	go func() {
		wg.Wait()
		for _, worker := range workers {
			close(worker.Done)
		}
		close(eventsCh)
	}()

	var events []step
	for ev := range eventsCh {
		events = append(events, ev)
	}
	return events
}

func buildScenarioSpecs() map[string]scenarioSpec {
	return map[string]scenarioSpec{
		"normal": {
			Key:   "normal",
			Title: "正常流程：2PC 成功提交",
			Intro: []string{
				"结构说明：上方为协调者服务，下方三个为参与者数据库。",
				"按 Enter 推进一步，查看当前状态与事件播报。",
			},
			Rule: scenarioRule{},
		},
		"a": {
			Key:   "a",
			Title: "故障A：参与者拒票",
			Intro: []string{"某参与者拒绝提交，导致全局 ABORT。"},
			Rule:  scenarioRule{RejectWorker: "数据库B"},
		},
		"b": {
			Key:   "b",
			Title: "故障B：参与者超时无响应",
			Intro: []string{"某参与者长时间沉默，协调者等待超时后回滚。"},
			Rule:  scenarioRule{TimeoutWorker: "数据库B"},
		},
		"c": {
			Key:   "c",
			Title: "故障C：协调者第一阶段崩溃",
			Intro: []string{"协调者在发送投票请求前宕机，参与者超时自回滚。"},
			Rule:  scenarioRule{CoordCrashBeforeVote: true},
		},
		"d": {
			Key:   "d",
			Title: "故障D：决议写入后崩溃",
			Intro: []string{"协调者已写入 COMMIT 决议，但广播前崩溃，恢复后重放。"},
			Rule:  scenarioRule{CoordCrashAfterCommit: true},
		},
	}
}

// buildWorkers 根据场景规则创建参与者，并注入投票行为（YES/NO/TIMEOUT）。
func buildWorkers(spec scenarioSpec) []*workerNode {
	names := []string{"数据库A", "数据库B", "数据库C"}
	workers := make([]*workerNode, 0, len(names))
	for _, name := range names {
		behavior := voteYes
		if spec.Rule.RejectWorker == name {
			behavior = voteNo
		}
		if spec.Rule.TimeoutWorker == name {
			behavior = voteTimeout
		}
		workers = append(workers, &workerNode{
			Name:       name,
			Behavior:   behavior,
			ReqCh:      make(chan voteRequest),
			DecisionCh: make(chan decisionMsg),
			Done:       make(chan struct{}),
		})
	}
	return workers
}

// runCoordinator 负责 2PC 两阶段的核心流程，并输出逐步事件。
func runCoordinator(state *clusterState, workers []*workerNode, spec scenarioSpec, eventsCh chan<- step) {
	emit := func(message string) {
		eventsCh <- state.snapshot(message)
	}

	state.setCoord(stateInit, "准备发起投票")
	state.setAllWorkers(stateInit, "待命")
	emit("初始化：所有参与者处于 INIT，等待协调者发起投票。")

	if spec.Rule.CoordCrashBeforeVote {
		state.setCoord(stateDown, "崩溃")
		emit("协调者宕机，投票请求未发出。")
		time.Sleep(80 * time.Millisecond)
		state.setAllWorkers(stateAbort, "超时")
		emit("参与者等待超时，自行 ABORT。")
		return
	}

	state.setCoord(stateWait, "等待投票")
	state.setAllWorkers(stateInit, "准备投票")
	emit("协调者发送 VOTE-REQ，进入 WAIT。")

	respCh := make(chan voteResponse, len(workers))
	for _, worker := range workers {
		req := voteRequest{Reply: make(chan voteResponse, 1)}
		worker.ReqCh <- req
		go func(w *workerNode, reply chan voteResponse) {
			if resp, ok := <-reply; ok {
				respCh <- resp
			}
		}(worker, req.Reply)
	}

	votes := make(map[string]voteBehavior, len(workers))
	timeout := time.After(220 * time.Millisecond)
	for len(votes) < len(workers) {
		select {
		case resp := <-respCh:
			votes[resp.Worker] = resp.Vote
		case <-timeout:
			goto doneVotes
		}
	}
doneVotes:

	var hasNo bool
	var hasTimeout bool
	for i, worker := range workers {
		vote, ok := votes[worker.Name]
		switch {
		case !ok:
			hasTimeout = true
			state.setWorker(i, stateInit, "无响应")
		case vote == voteNo:
			hasNo = true
			state.setWorker(i, stateAbort, "VOTE-NO")
		case vote == voteYes:
			state.setWorker(i, stateReady, "VOTE-YES")
		}
	}

	if hasNo {
		emit("投票结果：出现 NO，协调者准备全局回滚。")
	} else if hasTimeout {
		emit("投票结果：部分参与者无响应，等待超时。")
	} else {
		emit("投票结果：全部 YES，参与者进入 READY。")
	}

	if hasNo || hasTimeout {
		state.setCoord(stateAbort, "决定回滚")
		broadcastDecision(state, workers, stateAbort)
		emit("协调者广播 GLOBAL-ABORT，所有参与者回滚。")
		return
	}

	state.setCoord(stateCommit, "写入决议")
	emit("协调者写入全局决议 COMMIT。")

	if spec.Rule.CoordCrashAfterCommit {
		state.setCoord(stateDown, "宕机")
		emit("协调者崩溃，尚未广播决议。")
		time.Sleep(80 * time.Millisecond)
		state.setCoord(stateCommit, "恢复重放")
		broadcastDecision(state, workers, stateCommit)
		emit("协调者恢复，重放 GLOBAL-COMMIT。")
		return
	}

	broadcastDecision(state, workers, stateCommit)
	emit("协调者广播 GLOBAL-COMMIT，所有参与者提交完成。")
}

// runWorker 模拟参与者节点处理投票请求与最终决议。
func runWorker(state *clusterState, worker *workerNode) {
	for {
		select {
		case req := <-worker.ReqCh:
			if worker.Behavior == voteTimeout {
				// TIMEOUT 场景下不回复投票。
				continue
			}
			if worker.Behavior == voteNo {
				state.setWorkerByName(worker.Name, stateAbort, "VOTE-NO")
				req.Reply <- voteResponse{Worker: worker.Name, Vote: voteNo}
				continue
			}
			state.setWorkerByName(worker.Name, stateReady, "VOTE-YES")
			req.Reply <- voteResponse{Worker: worker.Name, Vote: voteYes}
		case decision := <-worker.DecisionCh:
			if decision.Decision == stateCommit {
				state.setWorkerByName(worker.Name, stateCommit, "提交完成")
			} else {
				state.setWorkerByName(worker.Name, stateAbort, "回滚")
			}
			close(decision.Ack)
		case <-worker.Done:
			return
		}
	}
}

func broadcastDecision(state *clusterState, workers []*workerNode, decision string) {
	acks := make([]chan struct{}, 0, len(workers))
	for _, worker := range workers {
		ack := make(chan struct{})
		acks = append(acks, ack)
		worker.DecisionCh <- decisionMsg{Decision: decision, Ack: ack}
	}
	for _, ack := range acks {
		<-ack
	}
}

func newClusterState() *clusterState {
	workers := []actorView{
		{Name: "数据库A", Role: roleWorker, State: stateInit, Note: "待命"},
		{Name: "数据库B", Role: roleWorker, State: stateInit, Note: "待命"},
		{Name: "数据库C", Role: roleWorker, State: stateInit, Note: "待命"},
	}
	return &clusterState{
		coord:   actorView{Name: "协调者", Role: roleCoord, State: stateInit, Note: "准备发起投票"},
		workers: workers,
	}
}

func (c *clusterState) setCoord(state, note string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.coord.State = state
	c.coord.Note = note
}

func (c *clusterState) setWorker(index int, state, note string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workers[index].State = state
	c.workers[index].Note = note
}

func (c *clusterState) setWorkerByName(name, state, note string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.workers {
		if c.workers[i].Name == name {
			c.workers[i].State = state
			c.workers[i].Note = note
			return
		}
	}
}

func (c *clusterState) setAllWorkers(state, note string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.workers {
		c.workers[i].State = state
		c.workers[i].Note = note
	}
}

func (c *clusterState) snapshot(message string) step {
	c.mu.Lock()
	defer c.mu.Unlock()
	workers := make([]actorView, len(c.workers))
	copy(workers, c.workers)
	return step{
		Event:   message,
		Coord:   c.coord,
		Workers: workers,
	}
}

func renderStep(title string, st step, index, total int) {
	fmt.Println("------------------------------------------------------------")
	fmt.Printf("%s | 步骤 %d/%d\n", title, index, total)
	fmt.Printf("事件：%s\n\n", st.Event)
	renderLayout(st.Coord, st.Workers)
}

func renderLayout(coord actorView, workers []actorView) {
	if len(workers) != 3 {
		fmt.Println("[渲染错误] 需要 3 个参与者")
		return
	}
	coordBox := makeBox(coord, 32)
	for _, line := range coordBox {
		fmt.Println(line)
	}
	fmt.Println()
	workerBoxes := [][]string{
		makeBox(workers[0], 24),
		makeBox(workers[1], 24),
		makeBox(workers[2], 24),
	}
	for i := 0; i < len(workerBoxes[0]); i++ {
		fmt.Printf("%s  %s  %s\n", workerBoxes[0][i], workerBoxes[1][i], workerBoxes[2][i])
	}
	fmt.Println()
}

func makeBox(actor actorView, width int) []string {
	contentWidth := width - 2
	lines := []string{
		actor.Name,
		fmt.Sprintf("Role: %s", actor.Role),
		fmt.Sprintf("State: %s", actor.State),
		fmt.Sprintf("Note: %s", actor.Note),
	}
	for i, line := range lines {
		lines[i] = padRight(line, contentWidth)
	}
	box := []string{
		"+" + strings.Repeat("-", contentWidth) + "+",
	}
	for _, line := range lines {
		box = append(box, "|"+line+"|")
	}
	box = append(box, "+"+strings.Repeat("-", contentWidth)+"+")
	return box
}

func padRight(text string, width int) string {
	runes := []rune(text)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return text + strings.Repeat(" ", width-len(runes))
}

func waitForEnter(prompt string) {
	fmt.Println(prompt)
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
}
