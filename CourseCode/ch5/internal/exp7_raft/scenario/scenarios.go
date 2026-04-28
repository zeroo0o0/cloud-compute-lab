package scenario

import (
	"fmt"

	"ch5/internal/exp7_raft/core"
	"ch5/internal/exp7_raft/utils"
)

func RunScenario(s core.Scenario, seed int64) (core.Report, error) {
	switch s {
	case core.ScenarioLeaderFailover:
		return runLeaderFailover(seed)
	default:
		return core.Report{}, fmt.Errorf("unknown scenario: %s", s)
	}
}

func runLeaderFailover(seed int64) (core.Report, error) {
	eng := core.NewEngine(core.DefaultConfig(), seed)
	report, err := eng.RunLeaderFailover()
	if err != nil {
		return core.Report{}, err
	}

	renderLeaderFailover(report)
	return report, nil
}

func renderLeaderFailover(r core.Report) {
	utils.RenderTitle("实验七：Raft 领导者选举（Leader 崩溃后自动故障转移）")
	utils.RenderLine("[Scene] 节点 Node-1 / Node-2 / Node-3 启动，初始全部为 Follower")
	for _, line := range r.Timeline {
		utils.RenderLine(line)
	}
	utils.RenderLine(fmt.Sprintf("[Result] 初始 Leader=Node-%d, 故障后 Leader=Node-%d, Final Term=%d", r.InitialLeader, r.NewLeader, r.FinalTerm))
}
