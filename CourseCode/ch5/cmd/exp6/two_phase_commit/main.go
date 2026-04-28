package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"ch5/internal/exp6_2pc"
)

func main() {
	scenario := flag.String("scenario", "all", "场景：normal | a | b | c | d | all")
	stepMS := flag.Int("step-ms", 650, "剧情演绎模式每句对白之间的间隔毫秒，0 表示不延迟")
	dataDir := flag.String("data-dir", filepath.FromSlash("./data/exp6_2pc"), "稳定存储日志目录")
	flag.Parse()

	exp6_2pc.SetVisualStepDelay(*stepMS)

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		fmt.Printf("创建数据目录失败: %v\n", err)
		os.Exit(1)
	}

	if *scenario == "all" {
		// 阶段一：正常场景
		fmt.Println("--- 阶段一：演示正常 2PC 提交流程 ---")
		runOne(exp6_2pc.ScenarioNormal, *dataDir)

		// 阶段二：故障场景（每个场景之间按 Enter 继续）
		faults := []exp6_2pc.Scenario{
			exp6_2pc.ScenarioWorkerReject,
			exp6_2pc.ScenarioWorkerTimeout,
			exp6_2pc.ScenarioCoordCrashPhase1,
			exp6_2pc.ScenarioCoordCrashPhase2,
		}
		fmt.Println("--- 阶段二：演示 a, b, c, d 四种故障场景（每个场景按 Enter 继续） ---")
		for i, s := range faults {
			waitForEnter(fmt.Sprintf("\n--- 按 Enter 继续，开始故障场景 %s（%d/%d）---", s, i+1, len(faults)))
			runOne(s, *dataDir)
		}
		return
	}

	runOne(exp6_2pc.Scenario(*scenario), *dataDir)
}

func runOne(s exp6_2pc.Scenario, dataDir string) {
	fmt.Println("============================================================")
	fmt.Printf("开始场景: %s\n", s)
	_, err := exp6_2pc.RunScenario(s, dataDir)
	if err != nil {
		fmt.Printf("场景执行失败: %v\n", err)
		os.Exit(1)
	}
}

func waitForEnter(prompt string) {
	fmt.Println(prompt)
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadString('\n')
}
