package main

import (
	"fmt"
	"sync"
	"time"
)

type NPC struct {
	ID     int
	Active bool
}

type Result struct {
	PlayerID int
	Success  bool
}

func runOneRound(round int) int {
	npc := &NPC{ID: 999, Active: false}
	var wg sync.WaitGroup
	results := make(chan Result, 5)

	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(playerID int) {
			defer wg.Done()
			if !npc.Active {
				time.Sleep(10 * time.Millisecond)
				npc.Active = true
				fmt.Printf("[第%d轮][掉落] 玩家 %d 成功拿到唯一宝物\n", round, playerID)
				results <- Result{PlayerID: playerID, Success: true}
				return
			}
			fmt.Printf("[第%d轮][失败] 玩家 %d 抢慢了\n", round, playerID)
			results <- Result{PlayerID: playerID, Success: false}
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
	return successCount
}

func main() {
	fmt.Println("=== 实验二：临界区与数据竞争 / 无锁错误版 ===")
	fmt.Println("场景: 5 名玩家同时抢同一个 NPC 的唯一掉落。")
	fmt.Println("目标: 稳定复现 Race Condition，看到“唯一物品被重复领取”。")

	totalRounds := 3
	for round := 1; round <= totalRounds; round++ {
		successCount := runOneRound(round)
		fmt.Printf("[第%d轮统计] 成功领取人数 = %d\n", round, successCount)
		if successCount > 1 {
			fmt.Println("  -> 违反唯一性约束：同一件宝物被复制发放。")
		}
	}

	fmt.Println("\n[结论] 问题不在业务规则，而在“检查 + 修改”没有放进同一个受保护的临界区。")
	fmt.Println("[下一步] 运行 right 版本，观察 Mutex 如何把两个动作变成原子操作。")
}
