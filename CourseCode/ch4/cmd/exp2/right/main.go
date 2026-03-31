package main

import (
	"fmt"
	"sync"
	"time"
)

type NPC struct {
	ID     int
	Active bool
	mu     sync.Mutex
}

type Result struct {
	PlayerID int
	Success  bool
}

func main() {
	fmt.Println("=== 实验二：临界区与数据竞争 / Mutex 修复版 ===")
	fmt.Println("场景: 仍然是 5 名玩家同时抢同一个 NPC 的唯一掉落。")
	fmt.Println("目标: 用互斥锁保护“检查 + 修改”，确保只有一个赢家。")

	npc := &NPC{ID: 999, Active: false}
	var wg sync.WaitGroup
	results := make(chan Result, 5)

	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(playerID int) {
			defer wg.Done()

			npc.mu.Lock()
			defer npc.mu.Unlock()

			if npc.Active {
				fmt.Printf("[失败] 玩家 %d 晚来一步，NPC %d 已消失。\n", playerID, npc.ID)
				results <- Result{PlayerID: playerID, Success: false}
				return
			}

			time.Sleep(10 * time.Millisecond)
			npc.Active = true
			fmt.Printf("[掉落] 玩家 %d 触发成功！NPC %d 送出宝物\n", playerID, npc.ID)
			results <- Result{PlayerID: playerID, Success: true}
		}(i)
	}

	wg.Wait()
	close(results)

	successCount := 0
	for result := range results {
		if result.Success {
			successCount++
		}
	}

	fmt.Printf("[统计] 成功领取人数 = %d\n", successCount)
	fmt.Println("[提示] 对照无锁版的多轮统计，观察这里是否还能出现重复掉落。")
}
