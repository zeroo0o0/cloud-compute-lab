package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
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

func runOneRound(round int, rng *rand.Rand) []Result {
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

			/*
				================ 【学生重点 实验二：无锁错误版】 ================
				请只看下面这段“先检查、后修改”：
				1. if !npc.Active：玩家先判断宝物是不是还在。
				2. time.Sleep(pickupCost)：模拟拾取动作需要一点时间。
				3. npc.Active = true：玩家最后才把宝物标记为“已经被拿走”。

				问题就在第 1 步和第 3 步之间：
				多个玩家可能同时看到 npc.Active 还是 false，于是都进入拾取流程。
				这就是《英雄集结演示实验.md》实验二要观察的“唯一宝物被重复领取”。
				==============================================================
			*/
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

	roundResults := make([]Result, 0, 5)
	for result := range results {
		roundResults = append(roundResults, result)
	}
	sort.Slice(roundResults, func(i, j int) bool {
		return roundResults[i].PlayerID < roundResults[j].PlayerID
	})
	return roundResults
}

func printTreasureSummary(round int, results []Result) {
	winners := make([]int, 0, len(results))
	for _, result := range results {
		if result.Success {
			winners = append(winners, result.PlayerID)
		}
	}

	fmt.Println(strings.Repeat("=", 48))
	fmt.Printf(">>> 第%d轮核心资源：唯一宝物归属\n", round)
	if len(winners) == 0 {
		fmt.Println(">>> 本轮没有任何玩家拿到宝物")
	} else if len(winners) == 1 {
		fmt.Printf(">>> 正常情况下应该只有 1 个赢家，本轮赢家：玩家 %d\n", winners[0])
	} else {
		fmt.Printf(">>> 发生竞态错误：宝物被重复领取，赢家变成了玩家 %v\n", winners)
	}
	fmt.Printf(">>> 成功领取人数 = %d\n", len(winners))
	fmt.Println(strings.Repeat("=", 48))
}

func pauseBetweenRounds(reader *bufio.Reader, round, total int, auto bool) {
	if round == total {
		return
	}
	if auto {
		time.Sleep(900 * time.Millisecond)
		return
	}
	fmt.Printf("[操作] 第%d轮结束。按回车进入下一轮对照...\n", round)
	_, _ = reader.ReadString('\n')
}

func main() {
	rounds := flag.Int("rounds", 5, "演示轮数")
	auto := flag.Bool("auto", false, "自动连续演示（默认每轮暂停等待回车）")
	flag.Parse()

	fmt.Println("=== 实验二：临界区与数据竞争 / 无锁错误版 ===")
	fmt.Println("场景: 5 名玩家同时抢同一个 NPC 的唯一掉落。")
	fmt.Println("目标: 逐轮观察竞态窗口，并把“唯一宝物”的最终归属单独高亮出来。")

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	reader := bufio.NewReader(os.Stdin)
	for round := 1; round <= *rounds; round++ {
		if !*auto {
			fmt.Printf("[操作] 准备开始第%d轮。按回车放开起跑线...\n", round)
			_, _ = reader.ReadString('\n')
		}

		results := runOneRound(round, rng)
		printTreasureSummary(round, results)
		pauseBetweenRounds(reader, round, *rounds, *auto)
	}

	fmt.Println("[提示] 再运行 right 版本，对比把“检查 + 修改”放进同一个互斥区后，核心资源是否还会重复发放。")
}
