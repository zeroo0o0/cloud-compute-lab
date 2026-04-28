package exp6_2pc

import (
	"ch5/internal/exp6_2pc/core"
	"ch5/internal/exp6_2pc/scenario"
	"ch5/internal/exp6_2pc/utils"
)

type PlanState = core.PlanState
type Scenario = core.Scenario
type Report = core.Report

const (
	StateINIT   = core.StateINIT
	StateWAIT   = core.StateWAIT
	StateREADY  = core.StateREADY
	StateCOMMIT = core.StateCOMMIT
	StateABORT  = core.StateABORT

	ScenarioNormal           = core.ScenarioNormal
	ScenarioWorkerReject     = core.ScenarioWorkerReject
	ScenarioWorkerTimeout    = core.ScenarioWorkerTimeout
	ScenarioCoordCrashPhase1 = core.ScenarioCoordCrashPhase1
	ScenarioCoordCrashPhase2 = core.ScenarioCoordCrashPhase2
)

func SetVisualStepDelay(ms int) {
	utils.SetVisualStepDelay(ms)
}

func RunScenario(s Scenario, rootDataDir string) (Report, error) {
	return scenario.RunScenario(s, rootDataDir)
}
