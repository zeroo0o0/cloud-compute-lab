package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// ── 节点状态 ─────────────────────────────────────────────────────────────────

type State string

const (
	Follower  State = "Follower"
	Candidate State = "Candidate"
	Leader    State = "Leader"
)

// ── Raft 节点结构体 ──────────────────────────────────────────────────────────

type Node struct {
	mu sync.Mutex

	ID          int
	state       State
	currentTerm int
	votedFor    int   // -1 表示本任期未投票
	leaderID    int   // 当前已知的 Leader ID，-1 表示未知
	peers       []string

	// 用于通知主循环重置选举超时（投票成功 / 收到心跳时触发）
	resetTimerCh chan struct{}
}

// ── 日志输出 ─────────────────────────────────────────────────────────────────

// log 在持锁状态下调用，直接读取 n 的字段
func (n *Node) log(format string, args ...interface{}) {
	prefix := fmt.Sprintf("[Node %d][Term %d][%s] ", n.ID, n.currentTerm, n.state)
	fmt.Printf(prefix+format+"\n", args...)
}

// ── 选举超时 ─────────────────────────────────────────────────────────────────

const (
	minElectionTimeout = 150 * time.Millisecond
	maxElectionTimeout = 300 * time.Millisecond
)

func randomElectionTimeout() time.Duration {
	rangeMS := int((maxElectionTimeout - minElectionTimeout).Milliseconds())
	return minElectionTimeout + time.Duration(rand.Intn(rangeMS+1))*time.Millisecond
}

// ── Raft 主循环 ──────────────────────────────────────────────────────────────

func (n *Node) run() {
	const (
		tickInterval      = 10 * time.Millisecond
		heartbeatInterval = 50 * time.Millisecond
	)

	electionTimeout := randomElectionTimeout()
	remaining := electionTimeout

	tick := time.NewTicker(tickInterval)
	heartbeat := time.NewTicker(heartbeatInterval)
	defer tick.Stop()
	defer heartbeat.Stop()

	n.mu.Lock()
	n.log("started")
	n.mu.Unlock()

	for {
		select {

		// ── 收到重置信号（投票成功 / 收到心跳） ─────────────────────
		case <-n.resetTimerCh:
			electionTimeout = randomElectionTimeout()
			remaining = electionTimeout

		// ── 选举超时检测 ──────────────────────────────────────────
		case <-tick.C:
			n.mu.Lock()
			state := n.state
			n.mu.Unlock()

			if state == Leader {
				continue
			}

			remaining -= tickInterval
			if remaining > 0 {
				continue
			}

			// ── 超时，发起选举 ───────────────────────────────────
			n.startElection()

			// 重置超时（无论选举成败）
			electionTimeout = randomElectionTimeout()
			remaining = electionTimeout

		// ── Leader 心跳 ──────────────────────────────────────────
		case <-heartbeat.C:
			n.mu.Lock()
			if n.state != Leader {
				n.mu.Unlock()
				continue
			}
			term := n.currentTerm
			peers := n.peers
			n.mu.Unlock()

			for _, peer := range peers {
				n.mu.Lock()
				n.log("send heartbeat to %s", peer)
				n.mu.Unlock()

				resp, err := n.sendAppendEntries(peer, AppendEntriesReq{
					Term:     term,
					LeaderID: n.ID,
				})
				if err != nil {
					continue // 连接失败，不崩溃
				}
				// 如果响应的 term 更高，说明有更新的 Leader，退回 Follower
				n.mu.Lock()
				if resp.Term > n.currentTerm {
					n.currentTerm = resp.Term
					n.state = Follower
					n.votedFor = -1
					n.leaderID = -1
					n.log("discovered higher Term %d, step down to Follower", resp.Term)
					n.mu.Unlock()
					break
				}
				n.mu.Unlock()
			}
		}
	}
}

// ── 选举流程 ─────────────────────────────────────────────────────────────────

func (n *Node) startElection() {
	n.mu.Lock()
	n.state = Candidate
	n.currentTerm++
	n.votedFor = n.ID
	term := n.currentTerm
	peers := n.peers
	n.log("election timeout, start election")
	n.mu.Unlock()

	votes := 1 // 自己投自己
	totalNodes := len(peers) + 1

	for _, peer := range peers {
		n.mu.Lock()
		req := RequestVoteReq{
			Term:        term,
			CandidateID: n.ID,
		}
		n.mu.Unlock()

		resp, err := n.sendRequestVote(peer, req)
		if err != nil {
			// 连接失败，跳过该 peer
			continue
		}

		n.mu.Lock()

		// 如果收到更高任期，退回 Follower
		if resp.Term > term {
			n.currentTerm = resp.Term
			n.state = Follower
			n.votedFor = -1
			n.leaderID = -1
			n.log("discovered higher Term %d, step down to Follower", resp.Term)
			n.mu.Unlock()
			return
		}

		// 仅当仍为同一 term 的 Candidate 时计票
		if n.state == Candidate && n.currentTerm == term {
			if resp.VoteGranted {
				votes++
				n.log("vote granted by %s", peer)
			} else {
				n.log("vote denied by %s", peer)
			}
		} else {
			// 状态已变（可能收到了心跳），停止计票
			n.mu.Unlock()
			return
		}

		n.mu.Unlock()
	}

	// 检查是否获得多数票
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.state == Candidate && n.currentTerm == term {
		if votes > totalNodes/2 {
			n.state = Leader
			n.leaderID = n.ID
			n.log("elected as Leader with %d votes", votes)
		} else {
			n.log("election failed, got %d/%d votes", votes, totalNodes)
		}
	}
}
