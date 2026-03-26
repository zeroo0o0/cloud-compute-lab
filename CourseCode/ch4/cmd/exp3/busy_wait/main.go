package main

import (
	"fmt"
	"sync"
	"time"
)

type NPC struct {
	ID    int
	Items int
	mu    sync.Mutex
}

func playerTask(playerID int, npc *NPC, wg *sync.WaitGroup) {
	defer wg.Done()
	emptyRuns := 0

	for {
		npc.mu.Lock()
		if npc.Items > 0 {
			npc.Items--
			fmt.Printf("\n[成功] 玩家 %d 抢到宝物！结束交互。(总计白跑了 %d 次)\n", playerID, emptyRuns)
			npc.mu.Unlock()
			return
		}
		npc.mu.Unlock()
		emptyRuns++
	}
}

func main() {
	fmt.Println("=== 实验三：告别忙等 / 仅用互斥锁的错误版 ===")
	fmt.Println("场景: 宝物暂时为空，玩家不断重复 Lock -> Check -> Unlock。")
	fmt.Println("目标: 观察 CPU 被“忙等”白白消耗，虽然业务最终能成功。")

	npc := &NPC{ID: 999, Items: 0}
	var wg sync.WaitGroup
	start := time.Now()

	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go playerTask(i, npc, &wg)
	}

	time.Sleep(100 * time.Millisecond)

	fmt.Printf("\n[系统] 宝物补货前，玩家已经忙等了约 %v。\n", time.Since(start))
	fmt.Println("[系统] NPC 开始投放 3 个宝物...")
	npc.mu.Lock()
	npc.Items += 3
	npc.mu.Unlock()

	wg.Wait()
	fmt.Println("[结论] 问题不是锁不安全，而是“没有东西时还在疯狂轮询”。")
	fmt.Println("[下一步] 运行 cond_wait 版本，观察等待者如何真正睡眠并被唤醒。")
}
