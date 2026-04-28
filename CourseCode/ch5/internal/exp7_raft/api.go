package exp7_raft

import (
	"ch5/internal/exp7_raft/core"
	"ch5/internal/exp7_raft/scenario"
	"ch5/internal/exp7_raft/utils"
)

type Role = core.Role
type Scenario = core.Scenario
type Report = core.Report

const (
	RoleFollower  = core.RoleFollower
	RoleCandidate = core.RoleCandidate
	RoleLeader    = core.RoleLeader

	ScenarioLeaderFailover = core.ScenarioLeaderFailover
)

func SetVisualStepDelay(ms int) {
	utils.SetVisualStepDelay(ms)
}

func RunScenario(s Scenario, seed int64) (Report, error) {
	return scenario.RunScenario(s, seed)
}
