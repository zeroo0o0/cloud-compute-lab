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
// 参与者行为：
//   - 数据库A：正常投票 YES（但协调者已崩溃，无法收到投票请求）
//   - 数据库B：正常投票 YES（但协调者已崩溃，无法收到投票请求）
//   - 数据库C：正常投票 YES（但协调者已崩溃，无法收到投票请求）
//
// 教学要点：
//   - 协调者单点故障的影响
//   - 参与者的超时自保护机制
//
// 运行方式：
//   go run ./fault_c
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

// =============================================================================
// 类型定义
// =============================================================================

// actorView 表示节点的可视化状态
type actorView struct {
	Name  string
	Role  string
	State string
	Note  string
}

// step 表示一个步骤的状态快照
type step struct {
	Event   string
	Coord   actorView
	Workers []actorView
}

type voteBehavior string

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

// =============================================================================
// 状态常量
// =============================================================================

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

// =============================================================================
// 核心协议函数
// =============================================================================

// runCoordinator 故障C：协调者第一阶段崩溃
//
// 流程：INIT -> 崩溃（DOWN），投票请求未发出，参与者超时自行 ABORT
func runCoordinator(state *clusterState, workers []*workerNode, eventsCh chan<- step) {
	emit := func(message string) {
		eventsCh <- state.snapshot(message)
	}

	// 1. 初始化
	state.setCoord(stateInit, "准备发起投票")
	state.setAllWorkers(stateInit, "待命")
	emit("初始化：所有参与者处于 INIT，等待协调者发起投票。")

	// 2. 协调者崩溃，投票请求未发出
	state.setCoord(stateDown, "崩溃")
	emit("协调者宕机，投票请求未发出。")

	// 3. 等待，模拟参与者超时
	time.Sleep(80 * time.Millisecond)
	state.setAllWorkers(stateAbort, "超时")
	emit("参与者等待超时，自行 ABORT。")

	return
}

// runWorker 参与者节点，处理投票请求与最终决议
func runWorker(state *clusterState, worker *workerNode) {
	for {
		select {
		case req := <-worker.ReqCh:
			// 正常流程：回复 YES
			state.setWorkerByName(worker.Name, stateReady, "VOTE-YES")
			req.Reply <- voteResponse{Worker: worker.Name, Vote: worker.Behavior}
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

// broadcastDecision 向所有参与者广播决议，等待确认
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

// =============================================================================
// 渲染函数
// =============================================================================

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
	box := []string{"+" + strings.Repeat("-", contentWidth) + "+"}
	for _, line := range lines {
		box = append(box, "|"+line+"|")
	}
	box = append(box, "+"+strings.Repeat("-", contentWidth)+"+")
	return box
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
	width := 0
	for _, r := range text {
		width += runeWidth(r)
	}
	return width
}

func runeWidth(r rune) int {
	if isWideRune(r) {
		return 2
	}
	return 1
}

func isWideRune(r rune) bool {
	switch {
	case unicode.Is(unicode.Han, r):
		return true
	case unicode.Is(unicode.Hangul, r):
		return true
	case unicode.Is(unicode.Hiragana, r):
		return true
	case unicode.Is(unicode.Katakana, r):
		return true
	default:
		return false
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

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

func waitForEnter(prompt string) {
	fmt.Println(prompt)
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
}

// =============================================================================
// 主函数
// =============================================================================

func main() {
	title := "故障C：协调者第一阶段崩溃"

	fmt.Println("============================================================")
	fmt.Printf("场景：%s\n", title)
	fmt.Println("结构说明：上方为协调者服务，下方三个为参与者数据库。")
	fmt.Println("按 Enter 推进一步，查看当前状态与事件播报。")
	fmt.Println()
	fmt.Println("协调者在发送投票请求前宕机，参与者超时自回滚。")
	fmt.Println("故障模型：协调者单点故障（投票前崩溃）。")

	state := newClusterState()

	// 创建参与者（全部正常投票 YES，故障不在参与者，而在协调者）
	workers := []*workerNode{
		{Name: "数据库A", Behavior: voteYes, ReqCh: make(chan voteRequest), DecisionCh: make(chan decisionMsg), Done: make(chan struct{})},
		{Name: "数据库B", Behavior: voteYes, ReqCh: make(chan voteRequest), DecisionCh: make(chan decisionMsg), Done: make(chan struct{})},
		{Name: "数据库C", Behavior: voteYes, ReqCh: make(chan voteRequest), DecisionCh: make(chan decisionMsg), Done: make(chan struct{})},
	}

	for _, w := range workers {
		go runWorker(state, w)
	}

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

	var events []step
	for ev := range eventsCh {
		events = append(events, ev)
	}

	for i, ev := range events {
		waitForEnter(fmt.Sprintf("按 Enter 继续（步骤 %d/%d）...", i+1, len(events)))
		renderStep(title, ev, i+1, len(events))
	}
}
