package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════════════
// 核心概念：每个节点是一个独立的 goroutine，通过 channel 模拟网络 RPC 通信
// ═══════════════════════════════════════════════════════════════════════════════

// ── 消息类型：模拟 Raft 协议中的两种 RPC ─────────────────────────────────────

// RequestVoteMsg: 候选人向其他节点发送的"拉票请求"（RequestVote RPC）
type RequestVoteMsg struct {
	Term        int       // 候选人的当前任期
	CandidateID int       // 候选人 ID
	RespCh      chan bool // 用于返回投票结果（true=赞成票, false=反对票）
}

// AppendEntriesMsg: Leader 向其他节点发送的"心跳/日志复制"（AppendEntries RPC）
type AppendEntriesMsg struct {
	Term    int   // Leader 的当前任期
	Entries []int // 日志条目（本实验中为空，仅用于心跳）
	RespCh  chan bool
}

// ── 节点结构体：每个节点维护自己的 Raft 状态 ──────────────────────────────────

type role string

const (
	roleFollower  role = "Follower"
	roleCandidate role = "Candidate"
	roleLeader    role = "Leader"
)

type node struct {
	ID       int
	mu       sync.Mutex // 保护节点状态的并发访问
	Role     role
	Term     int
	VotedFor int  // 本任期内投票给了谁（0=未投票）
	Alive    bool // 节点是否存活

	// 消息通道：模拟网络通信
	requestVoteCh   chan RequestVoteMsg   // 接收 RequestVote RPC
	appendEntriesCh chan AppendEntriesMsg // 接收 AppendEntries RPC
	done            chan struct{}         // 用于终止节点 goroutine
}

type nodeSnapshot struct {
	ID       int
	Role     role
	Term     int
	VotedFor int
	Alive    bool
}

// ── 配置 ─────────────────────────────────────────────────────────────────────

type config struct {
	NodeCount          int
	Tick               time.Duration
	MinElectionTimeout time.Duration
	MaxElectionTimeout time.Duration
	HeartbeatInterval  time.Duration
	KillLeaderAfter    time.Duration
	MaxRounds          int
}

func defaultConfig() config {
	return config{
		NodeCount:          3,
		Tick:               25 * time.Millisecond,
		MinElectionTimeout: 150 * time.Millisecond,
		MaxElectionTimeout: 320 * time.Millisecond,
		HeartbeatInterval:  70 * time.Millisecond,
		KillLeaderAfter:    450 * time.Millisecond,
		MaxRounds:          260,
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// main() 函数：Raft 领导者选举算法的核心流程
// ═══════════════════════════════════════════════════════════════════════════════
func main() {
	// ── 第一步：解析命令行参数 ────────────────────────────────────────────────
	seed := flag.Int64("seed", 7, "随机种子（影响 election timeout）")
	killAfterMS := flag.Int("kill-after-ms", 450, "首任 Leader 当选后多久触发宕机模拟")
	flag.Parse()

	cfg := defaultConfig()
	cfg.KillLeaderAfter = time.Duration(*killAfterMS) * time.Millisecond

	// ── 第二步：初始化集群 ────────────────────────────────────────────────────
	// 为每个节点创建消息通道（模拟网络连接）
	// 节点 ID 从 1 开始，所以通道数组长度为 NodeCount+1
	nodes := make([]*node, cfg.NodeCount+1)
	for i := 1; i <= cfg.NodeCount; i++ {
		nodes[i] = &node{
			ID:              i,
			Role:            roleFollower,
			Alive:           true,
			requestVoteCh:   make(chan RequestVoteMsg),
			appendEntriesCh: make(chan AppendEntriesMsg),
			done:            make(chan struct{}),
		}
	}

	// 用于通知主函数"新 Leader 已当选"
	leaderCh := make(chan int, 8)

	// ── 第三步：启动所有节点 ──────────────────────────────────────────────────
	// 每个节点在独立的 goroutine 中运行，模拟分布式环境中的并发行为
	rng := rand.New(rand.NewSource(*seed))
	for i := 1; i <= cfg.NodeCount; i++ {
		go runNode(nodes[i], nodes, cfg, rng, leaderCh)
	}

	renderCluster("集群启动", "3 个节点已启动，全部为 Follower", nodes)
	waitForEnter("按 Enter 继续，等待首任 Leader 当选...")

	// ── 第四步：等待首任 Leader 当选 ──────────────────────────────────────────
	initialLeader := <-leaderCh
	renderCluster("首任 Leader 当选", fmt.Sprintf("Node-%d 当选为首任 Leader", initialLeader), nodes)
	waitForEnter("按 Enter 继续，准备模拟 Leader 宕机...")

	// ── 第五步：模拟 Leader 宕机 ──────────────────────────────────────────────
	time.Sleep(cfg.KillLeaderAfter)
	nodes[initialLeader].mu.Lock()
	nodes[initialLeader].Alive = false
	nodes[initialLeader].Role = roleFollower
	nodes[initialLeader].mu.Unlock()
	close(nodes[initialLeader].done)
	renderCluster("Leader 宕机", fmt.Sprintf("Leader Node-%d 已崩溃", initialLeader), nodes)
	waitForEnter("按 Enter 继续，等待新 Leader 当选...")

	// ── 第六步：等待新 Leader 当选（故障转移） ────────────────────────────────
	newLeader, ok := waitForNewLeader(leaderCh, initialLeader, 5*time.Second)
	if !ok {
		fmt.Println("[超时] 未能在规定时间内完成故障转移")
		return
	}
	finalTerm := getNodeTerm(nodes[newLeader])
	renderCluster("故障转移完成", fmt.Sprintf("Node-%d 当选为新 Leader（Term=%d）", newLeader, finalTerm), nodes)
	waitForEnter("按 Enter 结束演示...")
}

// ═══════════════════════════════════════════════════════════════════════════════
// 节点运行函数：每个节点在此函数中独立运行
// 核心逻辑：定时器驱动 + 消息处理（Select 多路复用）
// ═══════════════════════════════════════════════════════════════════════════════
func runNode(n *node, allNodes []*node, cfg config, rng *rand.Rand, leaderCh chan int) {
	// 为当前节点生成随机的选举超时时间（Raft 关键机制：避免同时发起选举）
	electionTimeout := randomElectionTimeout(rng, cfg.MinElectionTimeout, cfg.MaxElectionTimeout)
	electionTimer := time.NewTicker(cfg.Tick)
	heartbeatTimer := time.NewTicker(cfg.HeartbeatInterval)
	defer electionTimer.Stop()
	defer heartbeatTimer.Stop()

	for {
		select {

		// ── 处理投票请求（RequestVote RPC）── 其他节点正在拉票 ────────────
		case msg := <-n.requestVoteCh:
			msg.RespCh <- handleRequestVote(n, msg)

		// ── 处理心跳请求（AppendEntries RPC）── Leader 保活 ─────────────
		case msg := <-n.appendEntriesCh:
			ok, newTimeout := handleAppendEntries(n, msg, cfg, rng)
			msg.RespCh <- ok
			if ok {
				electionTimeout = newTimeout // 心跳成功，重置选举超时
			}

		// ── Leader 定期发送心跳 ─────────────────────────────────────────
		case <-heartbeatTimer.C:
			if n.Role == roleLeader {
				sendHeartbeats(n, allNodes)
			}

		// ── Follower/Candidate 选举超时检测 ────────────────────────────
		case <-electionTimer.C:
			if n.Role == roleLeader {
				continue
			}
			electionTimeout -= cfg.Tick
			if electionTimeout > 0 {
				continue
			}

			// 超时！发起新一轮选举
			n.mu.Lock()
			n.Role = roleCandidate
			n.Term++
			n.VotedFor = n.ID
			n.mu.Unlock()
			electionTimeout = randomElectionTimeout(rng, cfg.MinElectionTimeout, cfg.MaxElectionTimeout)

			// 向所有其他节点发送 RequestVote RPC，收集选票
			votes := 1 // 先投自己一票
			for _, peer := range allNodes {
				if peer == nil || peer.ID == n.ID || !getNodeAlive(peer) {
					continue
				}
				if sendRequestVote(n, peer) {
					votes++
				}
			}

			// 检查是否获得多数票（Raft 关键规则：多数派才能当选）
			aliveCount := countAliveNodes(allNodes)
			if votes > aliveCount/2 {
				n.mu.Lock()
				n.Role = roleLeader
				n.mu.Unlock()
				// 通知主函数：新 Leader 已当选
				select {
				case leaderCh <- n.ID:
				default:
				}
			}

		// ── 收到终止信号，退出节点 ─────────────────────────────────────
		case <-n.done:
			return
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RPC 处理函数
// ═══════════════════════════════════════════════════════════════════════════════

// handleRequestVote: 处理来自候选人的投票请求（RequestVote RPC）
// Raft 规则：
//  1. 如果请求的任期 < 当前任期 → 拒绝（过期的候选人）
//  2. 如果请求的任期 > 当前任期 → 更新任期，转为 Follower
//  3. 每个任期只能投一票（先到先得）
func handleRequestVote(n *node, msg RequestVoteMsg) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 规则1：任期过期，拒绝
	if msg.Term < n.Term {
		return false
	}

	// 规则2：发现更高任期 → 更新任期，转为 Follower
	if msg.Term > n.Term {
		n.Term = msg.Term
		n.VotedFor = 0
		n.Role = roleFollower
	}

	// 规则3：每个任期只能投一票（先到先得）
	if n.VotedFor == 0 || n.VotedFor == msg.CandidateID {
		n.VotedFor = msg.CandidateID
		return true // 投赞成票
	}
	return false // 已经投给别人了
}

// handleAppendEntries: 处理来自 Leader 的心跳/日志复制请求（AppendEntries RPC）
// Raft 规则：
//  1. 如果 Leader 的任期 < 当前任期 → 拒绝（过期的 Leader）
//  2. 否则接受心跳，重置选举超时，转为 Follower
func handleAppendEntries(n *node, msg AppendEntriesMsg, cfg config, rng *rand.Rand) (bool, time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 规则1：任期过期，拒绝
	if msg.Term < n.Term {
		return false, 0
	}

	// 接受心跳：更新任期，重置选举超时，转为 Follower
	n.Term = msg.Term
	n.Role = roleFollower
	newTimeout := randomElectionTimeout(rng, cfg.MinElectionTimeout, cfg.MaxElectionTimeout)
	return true, newTimeout
}

// sendRequestVote: 向指定节点发送投票请求（模拟 RequestVote RPC）
func sendRequestVote(candidate *node, peer *node) bool {
	if peer == nil {
		return false
	}
	peerTerm := getNodeTerm(peer)
	if !getNodeAlive(peer) || peerTerm > candidate.Term {
		return false
	}
	respCh := make(chan bool, 1)
	peer.requestVoteCh <- RequestVoteMsg{
		Term:        candidate.Term,
		CandidateID: candidate.ID,
		RespCh:      respCh,
	}
	return <-respCh
}

// sendHeartbeats: Leader 向所有节点发送心跳（模拟 AppendEntries RPC）
// 心跳是 Leader 维持权力的关键：定期告诉其他节点"我还活着"
func sendHeartbeats(leader *node, allNodes []*node) {
	for _, peer := range allNodes {
		if peer == nil || peer.ID == leader.ID || !getNodeAlive(peer) {
			continue
		}
		respCh := make(chan bool, 1)
		peer.appendEntriesCh <- AppendEntriesMsg{
			Term:    leader.Term,
			Entries: nil,
			RespCh:  respCh,
		}
		<-respCh
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 辅助函数
// ═══════════════════════════════════════════════════════════════════════════════

// randomElectionTimeout: 生成随机的选举超时时间
// Raft 关键设计：每个节点的超时时间不同，避免同时发起选举导致票数分散
func randomElectionTimeout(rng *rand.Rand, min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	rangeMS := int((max - min).Milliseconds())
	return min + time.Duration(rng.Intn(rangeMS+1))*time.Millisecond
}

// 安全访问节点状态的辅助函数
func getNodeTerm(n *node) int   { n.mu.Lock(); defer n.mu.Unlock(); return n.Term }
func getNodeAlive(n *node) bool { n.mu.Lock(); defer n.mu.Unlock(); return n.Alive }

// countAliveNodes: 统计存活节点数量
func countAliveNodes(nodes []*node) int {
	count := 0
	for _, n := range nodes {
		if n != nil && getNodeAlive(n) {
			count++
		}
	}
	return count
}

func waitForNewLeader(leaderCh chan int, exclude int, timeout time.Duration) (int, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case id := <-leaderCh:
			if id != exclude {
				return id, true
			}
		case <-deadline:
			return 0, false
		}
	}
}

func waitForEnter(prompt string) {
	fmt.Println(prompt)
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
}

func renderCluster(title, note string, nodes []*node) {
	snapshots := snapshotNodes(nodes)
	fmt.Println("============================================================")
	fmt.Println(title)
	fmt.Printf("事件：%s\n\n", note)
	if len(snapshots) == 0 {
		fmt.Println("[提示] 无可用节点")
		return
	}
	boxes := make([][]string, 0, len(snapshots))
	for _, s := range snapshots {
		boxes = append(boxes, makeNodeBox(s, 24))
	}
	maxLines := len(boxes[0])
	for i := 0; i < maxLines; i++ {
		lineParts := make([]string, 0, len(boxes))
		for _, box := range boxes {
			lineParts = append(lineParts, box[i])
		}
		fmt.Println(strings.Join(lineParts, "  "))
	}
	fmt.Println()
}

func snapshotNodes(nodes []*node) []nodeSnapshot {
	snapshots := make([]nodeSnapshot, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		n.mu.Lock()
		snapshots = append(snapshots, nodeSnapshot{
			ID:       n.ID,
			Role:     n.Role,
			Term:     n.Term,
			VotedFor: n.VotedFor,
			Alive:    n.Alive,
		})
		n.mu.Unlock()
	}
	return snapshots
}

func makeNodeBox(s nodeSnapshot, width int) []string {
	contentWidth := width - 2
	vote := "-"
	if s.VotedFor != 0 {
		vote = fmt.Sprintf("%d", s.VotedFor)
	}
	lines := []string{
		fmt.Sprintf("Node-%d", s.ID),
		fmt.Sprintf("Role: %s", s.Role),
		fmt.Sprintf("Term: %d", s.Term),
		fmt.Sprintf("Vote: %s", vote),
		fmt.Sprintf("Note: %s", nodeNote(s)),
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

func nodeNote(s nodeSnapshot) string {
	if !s.Alive {
		return "DOWN"
	}
	switch s.Role {
	case roleLeader:
		return "Leader"
	case roleCandidate:
		return "Target: Self"
	case roleFollower:
		if s.VotedFor != 0 {
			return fmt.Sprintf("Voted %d", s.VotedFor)
		}
	}
	return "-"
}

func padRight(text string, width int) string {
	runes := []rune(text)
	if len(runes) >= width {
		return string(runes[:width])
	}
	return text + strings.Repeat(" ", width-len(runes))
}
