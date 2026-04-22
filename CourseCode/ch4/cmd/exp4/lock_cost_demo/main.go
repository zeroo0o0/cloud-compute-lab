package main

import (
	"flag"
	"fmt"
	"runtime"
	"sync"
	"time"
)

type result struct {
	name     string
	total    int
	lockOps  int64
	duration time.Duration
}

//go:noinline
func monsterGold(playerID, monsterID int) int {
	return (playerID*17 + monsterID*31 + 7) & 3
}

func defaultWorkers() int {
	workers := runtime.GOMAXPROCS(0)
	if workers < 2 {
		return 2
	}
	if workers > 8 {
		return 8
	}
	return workers
}

func runLocalOnly(workers, opsPerWorker int) result {
	start := time.Now()
	totals := make([]int, workers)
	var wg sync.WaitGroup

	for playerID := 0; playerID < workers; playerID++ {
		wg.Add(1)
		go func(playerID int) {
			defer wg.Done()

			localGold := 0
			for monsterID := 0; monsterID < opsPerWorker; monsterID++ {
				localGold += monsterGold(playerID, monsterID)
			}
			totals[playerID] = localGold
		}(playerID)
	}

	wg.Wait()

	total := 0
	for _, value := range totals {
		total += value
	}
	runtime.KeepAlive(totals)

	return result{name: "1. 不共享：每个玩家只算自己的本地金币", total: total, lockOps: 0, duration: time.Since(start)}
}

func runLockEveryKill(workers, opsPerWorker int) result {
	start := time.Now()
	var mu sync.Mutex
	var wg sync.WaitGroup
	teamGold := 0
	var lockOps int64

	for playerID := 0; playerID < workers; playerID++ {
		wg.Add(1)
		go func(playerID int) {
			defer wg.Done()

			for monsterID := 0; monsterID < opsPerWorker; monsterID++ {
				/*
					================ 【学生重点 实验四：锁太碎的代价】 ================
					请只看这几行：
					每打一个怪，就 Lock 一次、改一次 teamGold、Unlock 一次。

					写法是正确的，但锁粒度太小。
					当很多玩家同时刷怪时，所有人都会挤在同一把 mu 前面排队。
					==============================================================
				*/
				mu.Lock()
				teamGold += monsterGold(playerID, monsterID)
				lockOps++
				mu.Unlock()
			}
		}(playerID)
	}

	wg.Wait()
	return result{name: "2. 锁太多：每次击杀都更新共享金币", total: teamGold, lockOps: lockOps, duration: time.Since(start)}
}

func runBatchThenLock(workers, opsPerWorker int) result {
	start := time.Now()
	var mu sync.Mutex
	var wg sync.WaitGroup
	teamGold := 0
	var lockOps int64

	for playerID := 0; playerID < workers; playerID++ {
		wg.Add(1)
		go func(playerID int) {
			defer wg.Done()

			localGold := 0
			for monsterID := 0; monsterID < opsPerWorker; monsterID++ {
				localGold += monsterGold(playerID, monsterID)
			}

			/*
				================ 【学生重点 实验四：减少加锁次数】 ================
				这里先在 goroutine 自己的本地变量 localGold 里累计。
				本地变量不共享，所以不用加锁。

				等一个玩家本轮刷怪统计结束后，只进入临界区一次，把结果合并到 teamGold。
				最终金币不变，但 Lock/Unlock 次数从“每只怪一次”变成“每个玩家一次”。
				==============================================================
			*/
			mu.Lock()
			teamGold += localGold
			lockOps++
			mu.Unlock()
		}(playerID)
	}

	wg.Wait()
	return result{name: "3. 锁变少：本地累计后再合并一次", total: teamGold, lockOps: lockOps, duration: time.Since(start)}
}

func printResult(r result, expected int) {
	status := "OK"
	if r.total != expected {
		status = "WRONG"
	}
	fmt.Printf("%-46s total=%9d  lock/unlock=%9d  耗时=%9s  [%s]\n",
		r.name, r.total, r.lockOps, r.duration.Round(time.Microsecond), status)
}

func main() {
	workers := flag.Int("workers", defaultWorkers(), "并发玩家数量")
	ops := flag.Int("ops", 800000, "每个玩家刷怪次数")
	flag.Parse()

	if *workers < 1 {
		fmt.Println("workers 必须大于 0")
		return
	}
	if *ops < 1 {
		fmt.Println("ops 必须大于 0")
		return
	}

	totalOps := *workers * *ops
	fmt.Println("=== 实验四：锁的代价极简演示 ===")
	fmt.Println("场景：多个玩家刷怪得到金币，最后都要得到同一个 teamGold。")
	fmt.Printf("参数：workers=%d, 每个玩家刷怪=%d，总操作=%d, GOMAXPROCS=%d\n\n",
		*workers, *ops, totalOps, runtime.GOMAXPROCS(0))

	localOnly := runLocalOnly(*workers, *ops)
	lockEveryKill := runLockEveryKill(*workers, *ops)
	batchThenLock := runBatchThenLock(*workers, *ops)

	fmt.Println("结果对比：")
	printResult(localOnly, localOnly.total)
	printResult(lockEveryKill, localOnly.total)
	printResult(batchThenLock, localOnly.total)

	fmt.Println()
	fmt.Printf("锁太多 / 本地累计后再合并：耗时约 %.1f 倍\n",
		float64(lockEveryKill.duration)/float64(batchThenLock.duration))
	fmt.Println("[结论] 锁不是不能用，而是要少用、慎用、缩小临界区；能在本地算完再合并，就不要每一步都抢同一把锁。")
}
