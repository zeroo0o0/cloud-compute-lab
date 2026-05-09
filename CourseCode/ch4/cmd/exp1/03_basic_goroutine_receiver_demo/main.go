package main

import (
	"fmt"
	"time"
)

type result struct {
	player string
	text   string
}

func main() {
	start := time.Now()
	fastInput := make(chan string, 1)
	slowInput := make(chan string, 1)
	results := make(chan result, 2)

	// 学生辅助：预先放入输入。这里不用 goroutine，避免把“造输入”和“并发收包”混在一起。
	fastInput <- "fast player input"
	slowInput <- "slow player input"

	fmt.Println("最小演示：每个输入源交给独立 goroutine 读取")
	fmt.Println("关键写法：go receive(\"slow\", slowInput, results) 和 go receive(\"fast\", fastInput, results)。")

	/*
		================ 【学生重点 3/6】Goroutine 解耦：最小原理版 ================

		请先只看下面两行 go receive(...)：
		1. slow 的等待交给 slow 自己的 goroutine。
		2. fast 的等待交给 fast 自己的 goroutine。

		这段代码想说明：
		主循环只负责分发任务，不亲自排队等待 slow，所以 fast 可以先把结果送回 results。
		==========================================================================
	*/
	go receive("slow", slowInput, results)
	go receive("fast", fastInput, results)

	fmt.Printf("[%.0fms] 主循环已经完成收包任务分发，可以继续做别的事情。\n", time.Since(start).Seconds()*1000)

	for i := 0; i < 2; i++ {
		got := <-results
		fmt.Printf("[%.0fms] 收到 %s: %s\n", time.Since(start).Seconds()*1000, got.player, got.text)
	}

	fmt.Println("结论：slow 仍然慢，但它只卡住自己的 goroutine，不会阻止 fast 先被处理。")
	fmt.Println("对应游戏问题：网络收包从主循环里拆出去，慢连接不再拖住其他连接。")
}

func receive(player string, input <-chan string, results chan<- result) {
	text := <-input
	// 学生辅助：保留 slow/fast 的耗时差异，用于观察 fast 是否能先返回。
	if player == "slow" {
		time.Sleep(500 * time.Millisecond)
	} else {
		time.Sleep(20 * time.Millisecond)
	}
	results <- result{player: player, text: text}
}
