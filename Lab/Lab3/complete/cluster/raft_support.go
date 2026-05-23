package cluster

import "fmt"

type Role string

const (
	Follower  Role = "follower"
	Candidate Role = "candidate"
	Leader    Role = "leader"
)

type LogEntry struct {
	Term    int
	Command string
}

type Node struct {
	ID          string
	Term        int
	Role        Role
	VotedFor    string
	Log         []LogEntry
	CommitIndex int
}

func NewNode(id string) *Node {
	return &Node{ID: id, Role: Follower, CommitIndex: -1}
}

func (n *Node) LastLogIndex() int { return len(n.Log) - 1 }
func (n *Node) LastLogTerm() int {
	if len(n.Log) == 0 {
		return 0
	}
	return n.Log[len(n.Log)-1].Term
}

func (n *Node) HandleRequestVote(term int, candidateID string, lastLogIndex, lastLogTerm int) bool {
	if term < n.Term {
		return false
	}
	if term > n.Term {
		n.Term = term
		n.Role = Follower
		n.VotedFor = ""
	}
	if n.VotedFor != "" && n.VotedFor != candidateID {
		return false
	}
	if !isCandidateLogUpToDate(lastLogIndex, lastLogTerm, n.LastLogIndex(), n.LastLogTerm()) {
		return false
	}
	n.VotedFor = candidateID
	return true
}

func (n *Node) StartElection(peers []*Node) bool {
	n.Term++
	n.Role = Candidate
	n.VotedFor = n.ID
	votes := 1
	for _, peer := range peers {
		if peer.HandleRequestVote(n.Term, n.ID, n.LastLogIndex(), n.LastLogTerm()) {
			votes++
		}
	}
	if votes > (len(peers)+1)/2 {
		n.Role = Leader
		return true
	}
	return false
}

func (n *Node) HandleAppendEntries(term int, leaderID string, prevLogIndex, prevLogTerm int, entries []LogEntry, leaderCommit int) bool {
	if term < n.Term {
		return false
	}
	if term > n.Term {
		n.Term = term
	}
	n.Role = Follower
	n.VotedFor = leaderID
	if prevLogIndex >= 0 {
		if prevLogIndex >= len(n.Log) {
			return false
		}
		if n.Log[prevLogIndex].Term != prevLogTerm {
			return false
		}
	}
	insert := prevLogIndex + 1
	for i, entry := range entries {
		idx := insert + i
		if idx < len(n.Log) {
			if n.Log[idx].Term != entry.Term || n.Log[idx].Command != entry.Command {
				n.Log = append(append([]LogEntry(nil), n.Log[:idx]...), entries[i:]...)
				goto commit
			}
			continue
		}
		n.Log = append(n.Log, entries[i:]...)
		break
	}
commit:
	if leaderCommit > n.CommitIndex {
		lastIndex := len(n.Log) - 1
		if leaderCommit < lastIndex {
			n.CommitIndex = leaderCommit
		} else {
			n.CommitIndex = lastIndex
		}
	}
	return true
}

func (n *Node) Propose(command string, peers []*Node) (int, error) {
	if n.Role != Leader {
		return -1, fmt.Errorf("节点 %s 当前不是 leader", n.ID)
	}
	entry := LogEntry{Term: n.Term, Command: command}
	n.Log = append(n.Log, entry)
	index := len(n.Log) - 1
	prevIndex := index - 1
	prevTerm := 0
	if prevIndex >= 0 {
		prevTerm = n.Log[prevIndex].Term
	}
	acks := 1
	for _, peer := range peers {
		if peer.HandleAppendEntries(n.Term, n.ID, prevIndex, prevTerm, []LogEntry{entry}, n.CommitIndex) {
			acks++
		}
	}
	if acks <= (len(peers)+1)/2 {
		return -1, fmt.Errorf("命令 %q 未复制到多数节点", command)
	}
	n.CommitIndex = index
	for _, peer := range peers {
		_ = peer.HandleAppendEntries(n.Term, n.ID, index, entry.Term, nil, n.CommitIndex)
	}
	return index, nil
}

func isCandidateLogUpToDate(candidateIndex, candidateTerm, selfIndex, selfTerm int) bool {
	if candidateTerm != selfTerm {
		return candidateTerm > selfTerm
	}
	return candidateIndex >= selfIndex
}
