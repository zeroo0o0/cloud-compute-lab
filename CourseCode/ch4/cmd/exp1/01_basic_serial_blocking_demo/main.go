package main

import (
	"fmt"
	"time"
)

func formatMS(d time.Duration) string {
	return fmt.Sprintf("%.0fms", float64(d.Microseconds())/1000)
}

func main() {
	start := time.Now()

	fmt.Println("最小演示：串行读取两个输入源")
	fmt.Println("slow 读取要 500ms，fast 读取只要 20ms。")
	fmt.Println("关键写法：主循环先调用 readSlowInput()，再调用 readFastInput()。")
	fmt.Println("说明：这个文件不使用 goroutine，只演示串行调用顺序本身会导致等待。")

	/*
		================ 【学生重点 1/6】卡顿传染：最小原理版 ================

		请先只看下面两行调用顺序：
		1. readSlowInput() 先执行：主循环先等慢输入。
		2. readFastInput() 后执行：快输入排在慢输入后面。

		这段代码想说明：
		fast 本身不慢，但主循环是串行执行的，所以 fast 会被前面的 slow 拖住。
		====================================================================
	*/
	msg := readSlowInput()
	fmt.Printf("[%.0fms] 先处理 slow: %s\n", time.Since(start).Seconds()*1000, msg)

	msg = readFastInput()
	fmt.Printf("[%.0fms] 再处理 fast: %s\n", time.Since(start).Seconds()*1000, msg)

	fmt.Println("结论：fast 读取很快，但它排在 slow 后面，所以必须等 slow 的 500ms 结束。")
	fmt.Println("对应游戏问题：一个慢玩家可以让同一帧里其他玩家的输入一起排队。")
}

func readSlowInput() string {
	// 学生辅助：用 500ms 模拟慢玩家输入读取耗时。
	time.Sleep(500 * time.Millisecond)
	return "slow player input"
}

func readFastInput() string {
	// 学生辅助：用 20ms 模拟快玩家输入读取耗时。
	time.Sleep(20 * time.Millisecond)
	return "fast player input"
}
