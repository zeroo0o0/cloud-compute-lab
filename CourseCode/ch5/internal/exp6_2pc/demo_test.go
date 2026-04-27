package exp6_2pc

import "testing"

func TestScenarioNormalCommit(t *testing.T) {
	tmp := t.TempDir()
	report, err := RunScenario(ScenarioNormal, tmp)
	if err != nil {
		t.Fatalf("RunScenario(normal) failed: %v", err)
	}
	if report.Decision != StateCOMMIT {
		t.Fatalf("expected decision COMMIT, got %s", report.Decision)
	}
	if report.WorkerStates["Worker-A"] != StateCOMMIT || report.WorkerStates["Worker-B"] != StateCOMMIT {
		t.Fatalf("workers should COMMIT, got: %+v", report.WorkerStates)
	}
}

func TestScenarioDRecoveryReplaysCommit(t *testing.T) {
	tmp := t.TempDir()
	report, err := RunScenario(ScenarioCoordCrashPhase2, tmp)
	if err != nil {
		t.Fatalf("RunScenario(d) failed: %v", err)
	}
	if report.Decision != StateCOMMIT {
		t.Fatalf("expected recovered decision COMMIT, got %s", report.Decision)
	}
	if report.WorkerStates["Worker-A"] != StateCOMMIT || report.WorkerStates["Worker-B"] != StateCOMMIT {
		t.Fatalf("workers should COMMIT after recovery replay, got: %+v", report.WorkerStates)
	}
}
