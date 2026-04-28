package main

import (
	"flag"
	"fmt"
	"os"

	"ch5/internal/exp7_raft"
)

func main() {
	scenario := flag.String("scenario", string(exp7_raft.ScenarioLeaderFailover), "场景：leader_failover")
	stepMS := flag.Int("step-ms", 350, "剧情演示每句间隔毫秒，0 表示不延迟")
	seed := flag.Int64("seed", 7, "随机种子（影响随机 election timeout）")
	flag.Parse()

	exp7_raft.SetVisualStepDelay(*stepMS)

	report, err := exp7_raft.RunScenario(exp7_raft.Scenario(*scenario), *seed)
	if err != nil {
		fmt.Printf("场景执行失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("============================================================")
	fmt.Printf("演示完成: %s\n", report.Scenario)
	fmt.Printf("初始 Leader: Node-%d\n", report.InitialLeader)
	fmt.Printf("故障后 Leader: Node-%d\n", report.NewLeader)
	fmt.Printf("最终 Term: %d\n", report.FinalTerm)
}
