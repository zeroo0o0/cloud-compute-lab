// =============================================================================
// 场景：故障D：决议写入后崩溃（日志恢复重放）
// =============================================================================
//
// 故障模型：协调者在 Phase-2 崩溃（写入决议后、广播前）
//
// 预期行为：
//   Phase-1：全部 YES -> 协调者写入 COMMIT 到日志 -> 崩溃
//   所有参与者阻塞等待 -> 协调者重启 -> 从日志恢复决议 -> 重放 GLOBAL-COMMIT
//
// 教学要点：
//   - 决议持久化的重要性：若不写日志，崩溃后决议丢失，参与者永久阻塞
//   - 日志恢复机制：重启后从日志文件读取崩溃前的决议
//   - 2PC 的最终一致性保证：只要决议已持久化，协议最终一定能完成
//
// 运行方式：go run ./fault_d
// =============================================================================
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"
)

// ═══════════════════════════════════════════════════════════════════════════════
// main() 程序入口
// ═══════════════════════════════════════════════════════════════════════════════

func main() {
	title := "故障D：决议写入后崩溃（日志恢复重放）"

	fmt.Println("============================================================")
	fmt.Printf("场景：%s\n", title)
	fmt.Println("结构说明：上方为协调者服务，下方三个为参与者数据库。")
	fmt.Println("按 Enter 推进一步，查看当前状态与事件播报。")
	fmt.Println()
	fmt.Println("协调者崩溃后，所有参与者将阻塞等待。协调者重启后通过读取日志文件恢复决议并重放。")
	fmt.Println("故障模型：协调者在 Phase-2 崩溃（写入决议后、广播前）。")

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

// runCoordinator 协调者：故障D - 决议写入后崩溃，通过日志恢复重放
//
//	INIT -> 投票 -> 全部YES -> 写入COMMIT到日志文件 -> 崩溃
//	-> 参与者阻塞等待 -> 重启 -> 读取日志恢复 -> 重放GLOBAL-COMMIT
func runCoordinator(state *clusterState, workers []*workerNode, eventsCh chan<- step) {
	emit := func(msg string) { eventsCh <- state.snapshot(msg) }

	// ── Phase-1：投票阶段 ──────────────────────────────────────────────────

	state.setCoord(stateInit, "准备发起投票")
	state.setAllWorkers(stateInit, "待命")
	emit("初始化：所有参与者处于 INIT，等待协调者发起投票。")

	state.setCoord(stateWait, "等待投票")
	state.setAllWorkers(stateInit, "准备投票")
	emit("协调者发送 VOTE-REQ，进入 WAIT。")

	// 收集投票
	respCh := make(chan voteResponse, len(workers))
	for _, w := range workers {
		req := voteRequest{Reply: make(chan voteResponse, 1)}
		w.ReqCh <- req
		go func(wn *workerNode, ch chan voteResponse) {
			if r, ok := <-ch; ok {
				respCh <- r
			}
		}(w, req.Reply)
	}
	votes := make(map[string]voteBehavior, len(workers))
	for len(votes) < len(workers) {
		r := <-respCh
		votes[r.Worker] = r.Vote
	}
	for i, w := range workers {
		if votes[w.Name] == voteYes {
			state.setWorker(i, stateReady, "VOTE-YES")
		}
	}
	emit("投票结果：全部 YES，参与者进入 READY，等待协调者下发决议。")

	// ── Phase-2：写入日志并崩溃 ────────────────────────────────────────────

	// 定位源码目录，日志写入 fault_d/ 下
	_, srcFile, _, _ := runtime.Caller(0)
	logPath := filepath.Join(filepath.Dir(srcFile), "coordinator.log")

	workerNames := make([]string, len(workers))
	for i, w := range workers {
		workerNames[i] = w.Name
	}

	// 结构化日志：决议、阶段、参与者、状态、时间戳
	logContent := fmt.Sprintf(
		"DECISION=COMMIT\nPHASE=2\nWORKERS=%s\nSTATUS=PENDING_BROADCAST\nTIMESTAMP=%s\n",
		strings.Join(workerNames, ","),
		time.Now().Format("2006-01-02 15:04:05"),
	)

	// 写入日志文件
	state.setCoord(stateCommit, "写入决议+日志")
	if err := os.WriteFile(logPath, []byte(logContent), 0644); err != nil {
		emit(fmt.Sprintf("日志写入失败: %v", err))
		return
	}
	emit(fmt.Sprintf("协调者写入 COMMIT 决议到日志文件 [%s]", logPath))

	// 协调者崩溃
	state.setCoord(stateDown, "宕机")
	emit("协调者崩溃！决议已持久化但尚未广播。")

	// 参与者阻塞
	state.setAllWorkers(stateReady, "等待决议...")
	emit("所有参与者阻塞等待决议（无日志恢复将永久卡死）。")

	// ── 协调者重启，日志恢复 ──────────────────────────────────────────────

	state.setCoord(stateDown, "重启中")
	emit("协调者重启，读取日志文件恢复决议...")

	recovered, err := os.ReadFile(logPath)
	if err != nil {
		emit(fmt.Sprintf("日志读取失败: %v（无法恢复）", err))
		return
	}

	// 解析日志中的 DECISION
	recoveredStr := string(recovered)
	decision := "COMMIT"
	for _, line := range strings.Split(recoveredStr, "\n") {
		if strings.HasPrefix(line, "DECISION=") {
			decision = strings.TrimPrefix(line, "DECISION=")
			break
		}
	}

	state.setCoord(stateCommit, "日志恢复完成")
	emit(fmt.Sprintf("从日志恢复决议: %s，重放 GLOBAL-COMMIT。", decision))

	// 重放广播
	broadcastDecision(state, workers, stateCommit)
	emit("重放完成！所有参与者提交成功。")
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
