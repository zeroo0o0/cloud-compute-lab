package cluster

import (
	"errors"
	"fmt"
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
	for _, incoming := range remote {
		if incoming.NodeID == "" {
			return errors.New("gossip 消息中存在空节点 ID")
		}
		current, ok := t.Members[incoming.NodeID]
		if !ok || shouldReplaceState(current, incoming) {
			incoming.UpdatedRound = round
			t.Members[incoming.NodeID] = incoming
			continue
		}
		if incoming.NodeID == t.Self && incoming.Incarnation > current.Incarnation {
			current.Incarnation = incoming.Incarnation + 1
			current.Status = Alive
			current.Heartbeat = max(current.Heartbeat, incoming.Heartbeat)
			current.UpdatedRound = round
			t.Members[incoming.NodeID] = current
		}
	}
	return nil
}

func (t *Table) Targets(round int) []string {
	candidates := make([]string, 0, len(t.Members))
	for nodeID, state := range t.Members {
		if nodeID == t.Self || state.Status == Dead {
			continue
		}
		candidates = append(candidates, nodeID)
	}
	sort.Slice(candidates, func(i, j int) bool {
		left := hashGossipString(fmt.Sprintf("%s|%d|%s", t.Self, round, candidates[i]))
		right := hashGossipString(fmt.Sprintf("%s|%d|%s", t.Self, round, candidates[j]))
		if left == right {
			return candidates[i] < candidates[j]
		}
		return left < right
	})
	if len(candidates) > t.Fanout {
		candidates = candidates[:t.Fanout]
	}
	return append([]string(nil), candidates...)
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
