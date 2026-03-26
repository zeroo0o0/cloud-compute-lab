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
	cond  *sync.Cond
}

func playerGetTreasure(playerID int, npc *NPC, wg *sync.WaitGroup) {
	defer wg.Done()

	npc.mu.Lock()
	for npc.Items == 0 {
		fmt.Printf("[等待] 宝物为空，玩家 %d 陷入沉睡，释放锁...\n", playerID)
		npc.cond.Wait()
		fmt.Printf("[唤醒] 玩家 %d 被叫醒，重新检查宝物库存...\n", playerID)
	}

	npc.Items--
	fmt.Printf("[成功] 玩家 %d 取走宝物！剩余宝物数: %d\n", playerID, npc.Items)
	npc.mu.Unlock()
}

func npcRestock(npc *NPC, amount int) {
	npc.mu.Lock()
	npc.Items += amount
	fmt.Printf("\n[系统] NPC 补货了 %d 个宝物！\n", amount)
	npc.mu.Unlock()

	for i := 0; i < amount; i++ {
		npc.cond.Signal()
	}
}

func main() {
	fmt.Println("=== 实验三：告别忙等 / Cond 修复版 ===")
	fmt.Println("场景: 玩家在库存为空时不再轮询，而是进入等待队列。")
	fmt.Println("目标: 观察 Wait/Signal 的睡眠唤醒流程，并强调 for 重检条件。")

	npc := &NPC{ID: 999, Items: 0}
	npc.cond = sync.NewCond(&npc.mu)

	var wg sync.WaitGroup
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go playerGetTreasure(i, npc, &wg)
	}

	time.Sleep(50 * time.Millisecond)
	npcRestock(npc, 3)

	wg.Wait()
	fmt.Println("[结论] Wait 会先释放锁再休眠，被唤醒后重新抢锁并用 for 再次检查条件。")
	fmt.Println("[关键点] 即使出现虚假唤醒，for 也会把错误唤醒重新挡回去。")
}
