package cluster

/*
TODO A-4：Raft 元数据提交

这个文件负责“谁有资格修改集群元数据”。

在游戏中的作用：
1. 地图 owner / replica 属于集群元数据。
2. 节点故障时，handleNodeFailure 需要把副本提升为新主。
3. 这个 owner 变更不能由某个节点随便硬改，而要通过 Raft leader 提交日志。
4. commitPlacementLocked 会调用 Propose，把 owner 变更复制到多数节点后再生效。

你需要完成：
1. HandleRequestVote：处理投票请求，校验任期、投票记录和日志新旧。
2. StartElection：候选节点发起选举，获得多数票后成为 leader。
3. HandleAppendEntries：处理 leader 的日志复制和 commit 推进。
4. Propose：leader 追加命令，并复制到多数 follower。

关键要求：
1. 旧任期请求必须被拒绝。
2. 日志落后的候选人不能获得投票。
3. 只有多数节点复制成功，leader 才能推进 CommitIndex。
*/

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
	panic("[Lab3-A4] TODO 未实现：HandleRequestVote，需要校验任期、投票记录和候选人日志新旧")
}

func (n *Node) StartElection(peers []*Node) bool {
	panic("[Lab3-A4] TODO 未实现：StartElection，需要发起选举并统计多数票")
}

func (n *Node) HandleAppendEntries(term int, leaderID string, prevLogIndex, prevLogTerm int, entries []LogEntry, leaderCommit int) bool {
	panic("[Lab3-A4] TODO 未实现：HandleAppendEntries，需要完成日志匹配、追加和提交推进")
}

func (n *Node) Propose(command string, peers []*Node) (int, error) {
	return -1, fmt.Errorf("[Lab3-A4] TODO 未实现：Node.Propose，需要完成 Raft 多数日志复制与提交")
}

func isCandidateLogUpToDate(candidateIndex, candidateTerm, selfIndex, selfTerm int) bool {
	if candidateTerm != selfTerm {
		return candidateTerm > selfTerm
	}
	return candidateIndex >= selfIndex
}
