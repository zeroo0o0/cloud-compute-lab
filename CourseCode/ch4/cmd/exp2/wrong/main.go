package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type NPC struct {
	ID     int
	Active bool
}

type Result struct {
	PlayerID   int
	Success    bool
	ArriveLag  time.Duration
	PickupCost time.Duration
}

func runOneRound(round int, rng *rand.Rand) int {
	npc := &NPC{ID: 999, Active: false}
	var wg sync.WaitGroup
	results := make(chan Result, 5)
	startGate := make(chan struct{})

	for i := 1; i <= 5; i++ {
		arriveLag := time.Duration(rng.Intn(18)) * time.Millisecond
		pickupCost := time.Duration(6+rng.Intn(10)) * time.Millisecond
		wg.Add(1)
		go func(playerID int, arriveLag, pickupCost time.Duration) {
			defer wg.Done()

			<-startGate
			time.Sleep(arriveLag)

			if !npc.Active {
				fmt.Printf("[第%d轮] 玩家 %d 看到宝物仍在，准备拾取 (到达偏移=%s, 拾取耗时=%s)\n",
					round, playerID, arriveLag, pickupCost)
				time.Sleep(pickupCost)
				npc.Active = true
				fmt.Printf("[第%d轮][掉落] 玩家 %d 成功拿到唯一宝物\n", round, playerID)
				results <- Result{
					PlayerID:   playerID,
					Success:    true,
					ArriveLag:  arriveLag,
					PickupCost: pickupCost,
				}
				return
			}
			fmt.Printf("[第%d轮][失败] 玩家 %d 到达时宝物已经被别人改写\n", round, playerID)
			results <- Result{
				PlayerID:   playerID,
				Success:    false,
				ArriveLag:  arriveLag,
				PickupCost: pickupCost,
			}
		}(i, arriveLag, pickupCost)
	}

	close(startGate)
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
	fmt.Println("目标: 观察“检查宝物是否存在”和“标记宝物已被拿走”分离后，会出现重复掉落。")

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	totalRounds := 5
	for round := 1; round <= totalRounds; round++ {
		successCount := runOneRound(round, rng)
		fmt.Printf("[第%d轮统计] 成功领取人数 = %d\n", round, successCount)
		if successCount > 1 {
			fmt.Println("  现象: 本轮发生了重复掉落。")
		} else {
			fmt.Println("  现象: 本轮看起来正常，但这并不代表代码没有竞态窗口。")
		}
		fmt.Println()
	}

	fmt.Println("[提示] 再运行 right 版本，对比把“检查 + 修改”放进同一个互斥区后，统计结果会怎样变化。")
}
