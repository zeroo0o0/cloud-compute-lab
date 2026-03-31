package main

import (
	"fmt"
	"time"
)

const trackWidth = 20

type InputEvent struct {
	PlayerID int
	Action   string
	DeltaX   int
	Latency  time.Duration
}

func formatMS(d time.Duration) string {
	return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000.0)
}

func clamp(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func renderPositions(pos map[int]int) string {
	return fmt.Sprintf("P1(x=%d) P2(x=%d) P3(x=%d) P4(x=%d)", pos[1], pos[2], pos[3], pos[4])
}

func runFrameSingle(
	frame int,
	order []int,
	events map[int]InputEvent,
	positions map[int]int,
	budget time.Duration,
	dragHint bool,
) {
	fmt.Printf("\n[Frame %d] 开始收集输入...\n", frame)
	start := time.Now()

	for _, pid := range order {
		ev := events[pid]
		time.Sleep(ev.Latency)
		waited := time.Since(start)

		note := ""
		if pid == 4 {
			note = "  <- 卡顿源头"
		} else if dragHint {
			note = "  <- 流畅玩家被拖累"
		}

		fmt.Printf("  -> 收到玩家%d事件: %s(%+d) (累计等待%s)%s\n",
			ev.PlayerID, ev.Action, ev.DeltaX, formatMS(waited), note)

		positions[ev.PlayerID] = clamp(positions[ev.PlayerID]+ev.DeltaX, 0, trackWidth)
	}

	cost := time.Since(start)
	fmt.Printf("[Frame %d] 位置快照: %s\n", frame, renderPositions(positions))
	fmt.Printf("[Frame %d] 结束, 耗时%s (目标<100ms)\n", frame, formatMS(cost))

	if cost > budget {
		fmt.Println("  警告: 帧时间超标, 游戏体验卡顿")
		fmt.Println("  原因: 单线程串行收包, 慢事件阻塞了后续事件处理")
	}
}

func main() {
	fmt.Println("=== 实验一：突破单线程瓶颈 / 本地串行主循环版 ===")
	fmt.Println("场景: 4 名玩家同时上报输入，玩家4 处于“地铁断流”环境，延迟固定 500ms。")
	fmt.Println("目标: 观察单线程主循环为什么会把慢玩家的延迟传染给整帧。")

	positions := map[int]int{1: 2, 2: 6, 3: 10, 4: 14}

	frame1 := map[int]InputEvent{
		1: {PlayerID: 1, Action: "MOVE", DeltaX: +1, Latency: 11 * time.Millisecond},
		2: {PlayerID: 2, Action: "MOVE", DeltaX: +1, Latency: 12 * time.Millisecond},
		3: {PlayerID: 3, Action: "MOVE", DeltaX: -1, Latency: 13 * time.Millisecond},
		4: {PlayerID: 4, Action: "MOVE", DeltaX: -1, Latency: 500 * time.Millisecond},
	}
	runFrameSingle(1, []int{1, 2, 3, 4}, frame1, positions, 523*time.Millisecond, false)

	frame2 := map[int]InputEvent{
		1: {PlayerID: 1, Action: "MOVE", DeltaX: +1, Latency: 11 * time.Millisecond},
		2: {PlayerID: 2, Action: "MOVE", DeltaX: +1, Latency: 12 * time.Millisecond},
		3: {PlayerID: 3, Action: "MOVE", DeltaX: -1, Latency: 13 * time.Millisecond},
		4: {PlayerID: 4, Action: "MOVE", DeltaX: -1, Latency: 500 * time.Millisecond},
	}
	runFrameSingle(2, []int{4, 1, 2, 3}, frame2, positions, 523*time.Millisecond, true)

	fmt.Println("\n[提示] 再运行 network_serial_server_demo、network_goroutine_server_demo 或 network_event_driven_sync_demo，对比不同层次的解耦方式。")
}
