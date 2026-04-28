package core

import (
	"fmt"
	"math/rand"
	"sort"
	"time"
)

type Role string

type Scenario string

const (
	RoleFollower  Role = "Follower"
	RoleCandidate Role = "Candidate"
	RoleLeader    Role = "Leader"

	ScenarioLeaderFailover Scenario = "leader_failover"
)

type Report struct {
	Scenario      Scenario
	InitialLeader int
	NewLeader     int
	FinalTerm     int
	Timeline      []string
}

type Config struct {
	NodeCount           int
	Tick                time.Duration
	MinElectionTimeout  time.Duration
	MaxElectionTimeout  time.Duration
	HeartbeatInterval   time.Duration
	KillLeaderAfter     time.Duration
	MaxSimulationRounds int
}

type Node struct {
	ID               int
	Role             Role
	Term             int
	VotedFor         int
	Alive            bool
	electionTimeout  time.Duration
	electionElapsed  time.Duration
	heartbeatElapsed time.Duration
}

type Engine struct {
	cfg         Config
	rng         *rand.Rand
	nodes       []*Node
	elapsed     time.Duration
	initialID   int
	newID       int
	killed      bool
	killElapsed time.Duration
	timeline    []string
}

func DefaultConfig() Config {
	return Config{
		NodeCount:           3,
		Tick:                25 * time.Millisecond,
		MinElectionTimeout:  150 * time.Millisecond,
		MaxElectionTimeout:  320 * time.Millisecond,
		HeartbeatInterval:   60 * time.Millisecond,
		KillLeaderAfter:     420 * time.Millisecond,
		MaxSimulationRounds: 220,
	}
}

func NewEngine(cfg Config, seed int64) *Engine {
	rng := rand.New(rand.NewSource(seed))
	nodes := make([]*Node, 0, cfg.NodeCount)
	for i := 1; i <= cfg.NodeCount; i++ {
		n := &Node{ID: i, Role: RoleFollower, Alive: true}
		n.electionTimeout = randomTimeout(rng, cfg.MinElectionTimeout, cfg.MaxElectionTimeout)
		nodes = append(nodes, n)
	}
	return &Engine{cfg: cfg, rng: rng, nodes: nodes}
}

func (e *Engine) RunLeaderFailover() (Report, error) {
	e.logf("[Init] 3 个节点启动，全部处于 Follower，随机 election timeout 开始倒计时")

	for round := 0; round < e.cfg.MaxSimulationRounds; round++ {
		e.elapsed += e.cfg.Tick

		leader := e.currentLeader()
		if leader != nil {
			e.stepLeader(leader)
			e.tryKillLeader(leader)
		}

		e.stepFollowersAndCandidates()

		if e.initialID != 0 && e.killed && e.newID != 0 {
			finalLeader := e.currentLeader()
			if finalLeader == nil {
				continue
			}
			return Report{
				Scenario:      ScenarioLeaderFailover,
				InitialLeader: e.initialID,
				NewLeader:     e.newID,
				FinalTerm:     finalLeader.Term,
				Timeline:      append([]string(nil), e.timeline...),
			}, nil
		}
	}

	return Report{}, fmt.Errorf("simulation timeout: did not observe complete failover")
}

func (e *Engine) stepLeader(leader *Node) {
	leader.heartbeatElapsed += e.cfg.Tick
	if leader.heartbeatElapsed < e.cfg.HeartbeatInterval {
		return
	}
	leader.heartbeatElapsed = 0
	for _, n := range e.nodes {
		if !n.Alive || n.ID == leader.ID {
			continue
		}
		n.Role = RoleFollower
		n.Term = max(n.Term, leader.Term)
		n.electionElapsed = 0
	}
	e.logf("[Heartbeat] Leader Node-%d(term=%d) 广播心跳", leader.ID, leader.Term)
}

func (e *Engine) tryKillLeader(leader *Node) {
	if e.killed || e.initialID == 0 {
		return
	}
	e.killElapsed += e.cfg.Tick
	if e.killElapsed < e.cfg.KillLeaderAfter {
		return
	}
	leader.Alive = false
	leader.Role = RoleFollower
	e.killed = true
	e.logf("[Fault] Kill 当前 Leader Node-%d，模拟宕机", leader.ID)
}

func (e *Engine) stepFollowersAndCandidates() {
	for _, n := range e.nodes {
		if !n.Alive || n.Role == RoleLeader {
			continue
		}
		n.electionElapsed += e.cfg.Tick
		if n.electionElapsed >= n.electionTimeout {
			e.startElection(n)
		}
	}
}

func (e *Engine) startElection(c *Node) {
	c.Role = RoleCandidate
	c.Term++
	c.VotedFor = c.ID
	c.electionElapsed = 0
	c.electionTimeout = randomTimeout(e.rng, e.cfg.MinElectionTimeout, e.cfg.MaxElectionTimeout)

	votes := 1
	aliveCount := 0
	for _, n := range e.nodes {
		if n.Alive {
			aliveCount++
		}
	}

	for _, peer := range e.nodes {
		if !peer.Alive || peer.ID == c.ID {
			continue
		}
		if peer.Term > c.Term {
			continue
		}
		if peer.Term < c.Term {
			peer.Term = c.Term
			peer.VotedFor = 0
		}
		if peer.VotedFor == 0 || peer.VotedFor == c.ID {
			peer.VotedFor = c.ID
			peer.Role = RoleFollower
			peer.electionElapsed = 0
			votes++
		}
	}

	e.logf("[Election] Node-%d 发起选举(term=%d)，获得 %d/%d 票", c.ID, c.Term, votes, aliveCount)
	if votes <= aliveCount/2 {
		return
	}

	for _, n := range e.nodes {
		if !n.Alive {
			continue
		}
		if n.ID == c.ID {
			n.Role = RoleLeader
			n.heartbeatElapsed = 0
			continue
		}
		n.Role = RoleFollower
		n.electionElapsed = 0
	}

	if e.initialID == 0 {
		e.initialID = c.ID
		e.logf("[Leader] Node-%d 当选首任 Leader，Term=%d", c.ID, c.Term)
		return
	}

	if e.killed && e.newID == 0 {
		e.newID = c.ID
		e.logf("[Leader] Node-%d 在 Leader 宕机后当选新 Leader，Term=%d", c.ID, c.Term)
	}
}

func (e *Engine) currentLeader() *Node {
	for _, n := range e.nodes {
		if n.Alive && n.Role == RoleLeader {
			return n
		}
	}
	return nil
}

func (e *Engine) logf(format string, args ...any) {
	s := fmt.Sprintf(format, args...)
	e.timeline = append(e.timeline, s)
}

func randomTimeout(rng *rand.Rand, min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	rangeMs := int((max - min).Milliseconds())
	deltaMs := rng.Intn(rangeMs + 1)
	return min + time.Duration(deltaMs)*time.Millisecond
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (r Report) SortedTimeline() []string {
	out := append([]string(nil), r.Timeline...)
	sort.Strings(out)
	return out
}
