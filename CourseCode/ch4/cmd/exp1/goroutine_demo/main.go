package main

import (
	"fmt"
	"sync"
	"time"
)

type Message struct {
	PlayerID int
	Action   string
	DX       int
	DY       int
}

type Player struct {
	ID      int
	Latency time.Duration
}

func runSingleThreaded(players []Player) {
	fmt.Println("\n>>> [对照] 单线程版本：一人卡顿，全员排队")
	start := time.Now()

	for _, p := range players {
		time.Sleep(p.Latency)
		fmt.Printf("  [Frame 1] 玩家 %d 的输入处理完成 (耗时 %v)\n", p.ID, p.Latency)
	}

	total := time.Since(start)
	fmt.Printf("--- 结果：整帧耗时 %v (目标 <100ms: 失败) ---\n", total)
}

func runConcurrent(players []Player) {
	fmt.Println("\n>>> [改进] Goroutine 版本：每条连接独立处理")
	start := time.Now()
	var wg sync.WaitGroup

	for _, p := range players {
		wg.Add(1)
		go func(p Player) {
			defer wg.Done()
			time.Sleep(p.Latency)
			fmt.Printf("  [Async] 玩家 %d 响应成功 (实际网络延迟 %v)\n", p.ID, p.Latency)
		}(p)
	}

	fmt.Printf("  [Main] 主循环已完成任务分发，耗时: %v\n", time.Since(start))
	wg.Wait()
	fmt.Printf("--- 结果：主逻辑几乎瞬间解脱，流畅玩家不再排队 ---\n")
}

func runEventDriven(players []Player) {
	fmt.Println("\n>>> [补强] 事件驱动 + 增量同步：让主循环只处理已到达事件")
	eventQueue := make(chan Message, 10)
	done := make(chan bool)

	go func() {
		fmt.Println("  [Loop] 主循环启动，等待事件触发...")
		receivedCount := 0
		loopStart := time.Now()

		for {
			select {
			case msg := <-eventQueue:
				receivedCount++
				fmt.Printf("  [Render] 收到玩家 %d 增量位移 (%d, %d)，当前耗时: %v\n",
					msg.PlayerID, msg.DX, msg.DY, time.Since(loopStart))

				if receivedCount == len(players) {
					done <- true
					return
				}
			case <-time.After(600 * time.Millisecond):
				fmt.Println("  [Warn] 帧等待超时！")
				return
			}
		}
	}()

	for _, p := range players {
		go func(p Player) {
			time.Sleep(p.Latency)
			eventQueue <- Message{
				PlayerID: p.ID,
				Action:   "MOVE",
				DX:       1,
				DY:       0,
			}
		}(p)
	}

	<-done
	fmt.Println("--- 结果：事件驱动模型实现了按需处理，延迟被隔离在各自的协程中 ---")
}

func main() {
	fmt.Println("=== 实验一：突破单线程瓶颈（Goroutine 并发改造） ===")
	fmt.Println("场景: 使用同一组玩家输入，对比串行收包、goroutine 解耦和事件驱动三种处理方式。")
	fmt.Println("讲解重点: 慢连接隔离、主循环耗时、事件驱动与增量同步。")

	players := []Player{
		{ID: 1, Latency: 10 * time.Millisecond},
		{ID: 2, Latency: 15 * time.Millisecond},
		{ID: 3, Latency: 12 * time.Millisecond},
		{ID: 4, Latency: 500 * time.Millisecond},
	}

	runSingleThreaded(players)
	runConcurrent(players)
	runEventDriven(players)

	fmt.Println("\n[结论] goroutine 解决的是“慢连接拖住主逻辑”，事件驱动解决的是“只在事件到达时推进状态”。")
}
