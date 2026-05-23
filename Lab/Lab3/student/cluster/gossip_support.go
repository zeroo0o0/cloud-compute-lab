package cluster

/*
TODO A-2：Gossip 成员状态传播

这个文件负责“节点是否还活着”。

在游戏中的作用：
1. 集群会维护 node-a、node-b、node-c 的成员表。
2. 每一轮心跳会通过 Targets 选出少量节点传播状态，而不是全量广播。
3. Merge 用来合并其他节点传来的成员状态。
4. 节点故障后，成员状态需要从 alive / suspect 变为 dead。
5. handleNodeFailure 会根据 dead 状态触发地图副本提升。

你需要完成：
1. Table.Merge：合并远端成员摘要。
2. Table.Targets：按 fanout 选择本轮 Gossip 目标。

关键要求：
1. 不能选择自己作为 Gossip 目标。
2. 不能选择已经 dead 的节点作为目标。
3. 合并状态时按 incarnation、heartbeat、status 判断哪个状态更新。
4. 不要用旧 heartbeat 覆盖新 heartbeat。
*/

import (
	"errors"
	"hash/fnv"
	"sort"
)

type Status string

const (
	Alive   Status = "alive"
	Suspect Status = "suspect"
	Dead    Status = "dead"
)

type MemberState struct {
	NodeID       string
	Incarnation  int
	Heartbeat    int
	Status       Status
	UpdatedRound int
}

type Table struct {
	Self         string
	Fanout       int
	SuspectAfter int
	DeadAfter    int
	Members      map[string]MemberState
}

func NewTable(self string, peers []string, fanout int) *Table {
	if fanout < 1 {
		fanout = 1
	}
	t := &Table{Self: self, Fanout: fanout, SuspectAfter: 2, DeadAfter: 4, Members: make(map[string]MemberState)}
	t.Members[self] = MemberState{NodeID: self, Incarnation: 1, Heartbeat: 0, Status: Alive, UpdatedRound: 0}
	for _, peer := range peers {
		if peer == "" || peer == self {
			continue
		}
		t.Members[peer] = MemberState{NodeID: peer, Incarnation: 1, Heartbeat: 0, Status: Alive, UpdatedRound: 0}
	}
	return t
}

func (t *Table) Tick(round int) {
	self := t.Members[t.Self]
	self.Heartbeat++
	self.Status = Alive
	self.UpdatedRound = round
	t.Members[t.Self] = self
	for nodeID, state := range t.Members {
		if nodeID == t.Self || state.Status == Dead {
			continue
		}
		age := round - state.UpdatedRound
		switch {
		case age >= t.DeadAfter:
			state.Status = Dead
		case age >= t.SuspectAfter:
			state.Status = Suspect
		}
		t.Members[nodeID] = state
	}
}

func (t *Table) Digest() []MemberState {
	nodeIDs := make([]string, 0, len(t.Members))
	for nodeID := range t.Members {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	states := make([]MemberState, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		states = append(states, t.Members[nodeID])
	}
	return states
}

func (t *Table) Merge(round int, remote []MemberState) error {
	return errors.New("[Lab3-A2] TODO 未实现：Table.Merge，需要按 incarnation/heartbeat/status 合并 gossip 摘要")
}

func (t *Table) Targets(round int) []string {
	panic("[Lab3-A2] TODO 未实现：Table.Targets，需要按 fanout 选择非自身、非 dead 的 gossip 目标")
}

func (t *Table) State(nodeID string) (MemberState, bool) {
	state, ok := t.Members[nodeID]
	return state, ok
}

func shouldReplaceState(current, incoming MemberState) bool {
	if incoming.Incarnation != current.Incarnation {
		return incoming.Incarnation > current.Incarnation
	}
	if incoming.Heartbeat != current.Heartbeat {
		return incoming.Heartbeat > current.Heartbeat
	}
	return statusRank(incoming.Status) > statusRank(current.Status)
}

func statusRank(status Status) int {
	switch status {
	case Alive:
		return 0
	case Suspect:
		return 1
	case Dead:
		return 2
	default:
		return -1
	}
}

func hashGossipString(text string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	return h.Sum64()
}
