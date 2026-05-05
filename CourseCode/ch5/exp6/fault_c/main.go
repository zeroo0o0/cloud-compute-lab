// =============================================================================
// 场景：故障C：协调者第一阶段崩溃
// =============================================================================
//
// 故障模型：协调者单点故障（投票前崩溃）
//
// 预期行为：
//   - 协调者 INIT -> DOWN，投票请求未发出
//   - 参与者超时后自行 ABORT
//
// 运行方式：go run ./fault_c
// =============================================================================
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
)

// ═══════════════════════════════════════════════════════════════════════════════
// main() 程序入口
// ═══════════════════════════════════════════════════════════════════════════════

func main() {
	title := "故障C：协调者第一阶段崩溃"

	fmt.Println("============================================================")
	fmt.Printf("场景：%s\n", title)
	fmt.Println("结构说明：上方为协调者服务，下方三个为参与者数据库。")
	fmt.Println("按 Enter 推进一步，查看当前状态与事件播报。")
	fmt.Println()
	fmt.Println("故障模型：协调者单点故障（投票前崩溃）。")
	fmt.Println("协调者在发送投票请求前宕机，参与者超时自回滚。")

	state := newClusterState()
	workers := []*workerNode{
		{Name: "数据库A", Behavior: voteYes, ReqCh: make(chan voteRequest), DecisionCh: make(chan decisionMsg), Done: make(chan struct{})},
		{Name: "数据库B", Behavior: voteYes, ReqCh: make(chan voteRequest), DecisionCh: make(chan decisionMsg), Done: make(chan struct{})},
		{Name: "数据库C", Behavior: voteYes, ReqCh: make(chan voteRequest), DecisionCh: make(chan decisionMsg), Done: make(chan struct{})},
	}

	// 启动参与者 goroutine
	for _, w := range workers {
		go runWorker(state, w)
	}

	// 启动协调者 goroutine，收集事件序列
	eventsCh := make(chan step, 16)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runCoordinator(state, workers, eventsCh)
	}()
	go func() {
		wg.Wait()
		for _, w := range workers {
			close(w.Done)
		}
		close(eventsCh)
	}()

	// 逐步渲染事件
	var events []step
	for ev := range eventsCh {
		events = append(events, ev)
	}
	for i, ev := range events {
		waitForEnter(fmt.Sprintf("按 Enter 继续（步骤 %d/%d）...", i+1, len(events)))
		renderStep(title, ev, i+1, len(events))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 2PC 协议核心逻辑
// ═══════════════════════════════════════════════════════════════════════════════

// runCoordinator 故障C：协调者第一阶段崩溃
//
//	INIT -> 崩溃（DOWN），投票请求未发出，参与者超时自行 ABORT
func runCoordinator(state *clusterState, workers []*workerNode, eventsCh chan<- step) {
	emit := func(msg string) { eventsCh <- state.snapshot(msg) }

	// Phase-1：初始化
	state.setCoord(stateInit, "准备发起投票")
	state.setAllWorkers(stateInit, "待命")
	emit("初始化：所有参与者处于 INIT，等待协调者发起投票。")

	// 协调者崩溃，投票请求未发出
	state.setCoord(stateDown, "崩溃")
	emit("协调者宕机，投票请求未发出。")

	// 参与者等待超时，自行 ABORT
	time.Sleep(80 * time.Millisecond)
	state.setAllWorkers(stateAbort, "超时")
	emit("参与者等待超时，自行 ABORT。")

	return
}

// runWorker 参与者：处理投票请求与最终决议
func runWorker(state *clusterState, w *workerNode) {
	for {
		select {
		case req := <-w.ReqCh:
			state.setWorkerByName(w.Name, stateReady, "VOTE-YES")
			req.Reply <- voteResponse{Worker: w.Name, Vote: voteYes}
		case dec := <-w.DecisionCh:
			if dec.Decision == stateCommit {
				state.setWorkerByName(w.Name, stateCommit, "提交完成")
			} else {
				state.setWorkerByName(w.Name, stateAbort, "回滚")
			}
			close(dec.Ack)
		case <-w.Done:
			return
		}
	}
}

// broadcastDecision 向所有参与者广播决议，等待全部确认
func broadcastDecision(state *clusterState, workers []*workerNode, decision string) {
	acks := make([]chan struct{}, 0, len(workers))
	for _, w := range workers {
		ack := make(chan struct{})
		acks = append(acks, ack)
		w.DecisionCh <- decisionMsg{Decision: decision, Ack: ack}
	}
	for _, ack := range acks {
		<-ack
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 通信类型定义
// ═══════════════════════════════════════════════════════════════════════════════

type voteBehavior string

const (
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

// ═══════════════════════════════════════════════════════════════════════════════
// 集群状态管理
// ═══════════════════════════════════════════════════════════════════════════════

const (
	stateInit   = "INIT"
	stateWait   = "WAIT"
	stateReady  = "READY"
	stateCommit = "COMMIT"
	stateAbort  = "ABORT"
	stateDown   = "DOWN"

	roleCoord  = "Service"
	roleWorker = "Worker"
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

type clusterState struct {
	mu      sync.Mutex
	coord   actorView
	workers []actorView
}

func newClusterState() *clusterState {
	return &clusterState{
		coord: actorView{Name: "协调者", Role: roleCoord, State: stateInit, Note: "准备发起投票"},
		workers: []actorView{
			{Name: "数据库A", Role: roleWorker, State: stateInit, Note: "待命"},
			{Name: "数据库B", Role: roleWorker, State: stateInit, Note: "待命"},
			{Name: "数据库C", Role: roleWorker, State: stateInit, Note: "待命"},
		},
	}
}

func (c *clusterState) setCoord(state, note string) {
	c.mu.Lock()
	c.coord.State = state
	c.coord.Note = note
	c.mu.Unlock()
}

func (c *clusterState) setWorker(index int, state, note string) {
	c.mu.Lock()
	c.workers[index].State = state
	c.workers[index].Note = note
	c.mu.Unlock()
}

func (c *clusterState) setWorkerByName(name, state, note string) {
	c.mu.Lock()
	for i := range c.workers {
		if c.workers[i].Name == name {
			c.workers[i].State = state
			c.workers[i].Note = note
			break
		}
	}
	c.mu.Unlock()
}

func (c *clusterState) setAllWorkers(state, note string) {
	c.mu.Lock()
	for i := range c.workers {
		c.workers[i].State = state
		c.workers[i].Note = note
	}
	c.mu.Unlock()
}

func (c *clusterState) snapshot(message string) step {
	c.mu.Lock()
	defer c.mu.Unlock()
	ws := make([]actorView, len(c.workers))
	copy(ws, c.workers)
	return step{Event: message, Coord: c.coord, Workers: ws}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 渲染与显示
// ═══════════════════════════════════════════════════════════════════════════════

func renderStep(title string, st step, index, total int) {
	fmt.Println("------------------------------------------------------------")
	fmt.Printf("%s | 步骤 %d/%d\n", title, index, total)
	fmt.Printf("事件：%s\n\n", st.Event)
	renderLayout(st.Coord, st.Workers)
}

func renderLayout(coord actorView, workers []actorView) {
	for _, line := range makeBox(coord, 32) {
		fmt.Println(line)
	}
	fmt.Println()
	boxes := [][]string{
		makeBox(workers[0], 24),
		makeBox(workers[1], 24),
		makeBox(workers[2], 24),
	}
	for i := 0; i < len(boxes[0]); i++ {
		fmt.Printf("%s  %s  %s\n", boxes[0][i], boxes[1][i], boxes[2][i])
	}
	fmt.Println()
}

func makeBox(a actorView, width int) []string {
	cw := width - 2
	lines := []string{a.Name, fmt.Sprintf("Role: %s", a.Role), fmt.Sprintf("State: %s", a.State), fmt.Sprintf("Note: %s", a.Note)}
	for i, l := range lines {
		lines[i] = padRight(l, cw)
	}
	box := []string{"+" + strings.Repeat("-", cw) + "+"}
	for _, l := range lines {
		box = append(box, "|"+l+"|")
	}
	return append(box, "+"+strings.Repeat("-", cw)+"+")
}

func padRight(text string, width int) string {
	trimmed := trimToWidth(text, width)
	pad := width - displayWidth(trimmed)
	if pad <= 0 {
		return trimmed
	}
	return trimmed + strings.Repeat(" ", pad)
}

func trimToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range text {
		w := runeWidth(r)
		if used+w > width {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String()
}

func displayWidth(text string) int {
	w := 0
	for _, r := range text {
		w += runeWidth(r)
	}
	return w
}

func runeWidth(r rune) int {
	if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hangul, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
		return 2
	}
	return 1
}

func waitForEnter(prompt string) {
	if prompt != "" {
		fmt.Println(prompt)
	}
	bufio.NewReader(os.Stdin).ReadString('\n')
}
