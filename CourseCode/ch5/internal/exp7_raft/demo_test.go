package exp7_raft

import "testing"

func TestLeaderFailover(t *testing.T) {
	report, err := RunScenario(ScenarioLeaderFailover, 7)
	if err != nil {
		t.Fatalf("RunScenario failed: %v", err)
	}
	if report.InitialLeader == 0 {
		t.Fatalf("expected an initial leader, got 0")
	}
	if report.NewLeader == 0 {
		t.Fatalf("expected a new leader after failover, got 0")
	}
	if report.NewLeader == report.InitialLeader {
		t.Fatalf("expected leader to change after kill, got same node: %d", report.NewLeader)
	}
	if report.FinalTerm < 2 {
		t.Fatalf("expected final term >= 2, got %d", report.FinalTerm)
	}
}
