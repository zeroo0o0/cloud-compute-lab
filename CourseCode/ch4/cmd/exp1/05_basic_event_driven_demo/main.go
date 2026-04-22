package main

import (
	"fmt"
	"time"
)

type event struct {
	player string
	action string
}

func main() {
	events := make(chan event, 4)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// 学生辅助：制造两个事件，让主循环能看到“有事件”和“没事件”的区别。
	go func() {
		time.Sleep(250 * time.Millisecond)
		events <- event{player: "fast", action: "MOVE +1"}
		time.Sleep(260 * time.Millisecond)
		events <- event{player: "slow", action: "MOVE -1"}
	}()

	fmt.Println("最小演示：事件驱动主循环")
	fmt.Println("关键写法：每个 tick 只 drain 已经到达的事件；没有事件时不等待，直接进入下一 tick。")

	for tick := 1; tick <= 6; tick++ {
		<-ticker.C
		fmt.Printf("\nTick %d\n", tick)

		handled := 0
		for {
			/*
				================ 【学生重点 5/6】事件驱动 tick：最小原理版 ================

				请先只看这个 select/default：
				1. case ev := <-events：有事件到达，就处理事件。
				2. default：当前没有事件，不等待，直接进入下一 tick。

				这段代码想说明：
				服务器主循环按 tick 前进；没有输入事件时也不阻塞，所以慢玩家不会卡住整帧。
				==========================================================================
			*/
			select {
			case ev := <-events:
				handled++
				fmt.Printf("处理事件：%s %s\n", ev.player, ev.action)
			default:
				if handled == 0 {
					fmt.Println("没有事件：主循环继续前进，不阻塞等待。")
				}
				goto nextTick
			}
		}

	nextTick:
	}

	fmt.Println("\n结论：事件到了就处理，没事件也照常推进 tick。")
	fmt.Println("对应游戏问题：慢玩家只会晚更新自己的状态，不会让整帧停下来。")
}
