//修改了展示出每tick内的三个阶段的输出，增加了输入指令的提示和反馈，增强了用户体验和理解。每个阶段都有清晰的输出，展示了输入采集、逻辑计算和画面渲染的过程。
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// GameState 定义游戏世界的“绝对状态”
type GameState struct {
	PlayerX int
	PlayerY int
	Frame   int // 当前帧数
}

func main() {
	state := GameState{PlayerX: 0, PlayerY: 0, Frame: 0}

	fmt.Println("==================================================")
	fmt.Println("=== 游戏主循环 (Game Loop) ===")
	fmt.Println("操作说明：请输入 W(上) S(下) A(左) D(右) 并按回车发送。")
	fmt.Println("系统机制：世界每 5 秒刷新一次（Tick），观察终端打印的三个阶段。")
	fmt.Println("按 Ctrl+C 退出程序。")
	fmt.Println("==================================================")

	// 1. 创建带缓冲的 Channel 作为输入队列
	inputChan := make(chan string, 100)

	// 2. 独立协程：模拟客户端异步发送指令
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			text := strings.ToUpper(strings.TrimSpace(scanner.Text()))
			if text == "W" || text == "A" || text == "S" || text == "D" {
				inputChan <- text
				fmt.Printf("\n   [客户端] 发送指令: '%s' (已放入网络队列，等待下一个Tick结算)\n", text)
			} else {
				fmt.Printf("\n   [系统提示] 无效指令: '%s'，请输入 W/A/S/D\n", text)
			}
		}
	}()

	// 3. Tick 定时器：每 5 秒触发一次主循环
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// 4. 核心：Game Loop (游戏主循环)
	for {
		<-ticker.C // 等待 5 秒钟的“滴答”
		state.Frame++

		fmt.Printf("\n>>> 【Tick %03d 】 <<<\n", state.Frame)

		// ----------------------------------------------------
		// 阶段 1：输入采集 (Input)
		// ----------------------------------------------------
		fmt.Print("├─ [阶段 1: 输入采集]  ")
		cmd := "NONE"
	DrainLoop:
		for {
			select {
			case c := <-inputChan:
				cmd = c // 保留最后一次有效输入
			default:
				break DrainLoop
			}
		}
		fmt.Printf("最终采纳指令: [%s]\n", cmd)

		// ----------------------------------------------------
		// 阶段 2：逻辑计算 (Update)
		// ----------------------------------------------------
		fmt.Print("├─ [阶段 2: 逻辑计算]  ")
		oldX, oldY := state.PlayerX, state.PlayerY
		
		updateState(&state, cmd) // 调用计算逻辑
		
		if state.PlayerX == oldX && state.PlayerY == oldY {
			fmt.Println("计算结果: 玩家位置无变化")
		} else {
			fmt.Printf("计算结果: 发生位移，坐标从 (%d, %d) 变为 (%d, %d)\n", oldX, oldY, state.PlayerX, state.PlayerY)
		}

		// ----------------------------------------------------
		// 阶段 3：画面渲染 (Render)
		// ----------------------------------------------------
		fmt.Print("└─ [阶段 3: 画面渲染]  ")
		render(state, cmd)
		fmt.Println("--------------------------------------------------")
	}
}

// updateState 根据输入推进游戏世界
func updateState(state *GameState, cmd string) {
	switch cmd {
	case "W":
		state.PlayerY++
	case "S":
		state.PlayerY--
	case "A":
		state.PlayerX--
	case "D":
		state.PlayerX++
	}
}

// render 将当前世界的逻辑状态“投影”给玩家
func render(state GameState, cmd string) {
	// 在真实的引擎中，这里会调用 OpenGL/DirectX 画图
	// 在终端里，我们用一行高度格式化的文本代表“一帧画面”
	fmt.Printf("[当前绝对坐标 X: %d, Y: %d]\n", state.PlayerX, state.PlayerY)
}