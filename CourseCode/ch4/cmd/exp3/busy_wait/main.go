package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type NPC struct {
	ID    int
	Items int
	mu    sync.Mutex
}

func playerTask(playerID int, npc *NPC, emptyRuns []atomic.Int64, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		/*
			================ 【学生重点 实验三：忙等错误版】 ================
			请盯住这个循环：
			1. Lock：玩家抢锁进来看一眼库存。
			2. Check：如果 npc.Items 还是 0，说明没有宝物。
			3. Unlock：马上释放锁，然后立刻进入下一轮。

			库存为空时，这段代码不会睡觉，只会不断重复 Lock -> Check -> Unlock。
			所以输出里的“白跑次数”会一直涨，这就是实验三要演示的忙等。
			==============================================================
		*/
		npc.mu.Lock()
		if npc.Items > 0 {
			npc.Items--
			left := npc.Items
			npc.mu.Unlock()
			fmt.Printf("[成功] 玩家 %d 抢到宝物！结束交互。(白跑次数=%d, 剩余宝物=%d)\n",
				playerID, emptyRuns[playerID-1].Load(), left)
			return
		}
		npc.mu.Unlock()
		emptyRuns[playerID-1].Add(1)
	}
}

func printStatus(npc *NPC, emptyRuns []atomic.Int64) {
	npc.mu.Lock()
	items := npc.Items
	npc.mu.Unlock()

	var total int64
	fmt.Println("------ 当前状态（忙等版） ------")
	fmt.Printf("NPC 库存: %d\n", items)
	for idx := range emptyRuns {
		count := emptyRuns[idx].Load()
		total += count
		fmt.Printf("玩家 %d 白跑次数: %d\n", idx+1, count)
	}
	fmt.Printf("累计空转检查次数: %d\n", total)
	fmt.Println("------------------------------")
}

func applyRestock(npc *NPC, amount int) {
	npc.mu.Lock()
	npc.Items += amount
	current := npc.Items
	npc.mu.Unlock()
	fmt.Printf("[系统] NPC 补货 %d 个宝物，当前库存=%d\n", amount, current)
}

func printHelp() {
	fmt.Println("可用命令:")
	fmt.Println("  status       查看当前库存与白跑次数")
	fmt.Println("  restock N    补货 N 个宝物，例如 restock 3")
	fmt.Println("  quit         退出演示")
}

func main() {
	fmt.Println("=== 实验三：告别忙等 / 仅用互斥锁的错误版 ===")
	fmt.Println("场景: 宝物为空时，玩家不断重复 Lock -> Check -> Unlock。")
	fmt.Println("目标: 把“补货”交给操作者，先观察忙等如何持续吞掉 CPU。")
	printHelp()

	npc := &NPC{ID: 999, Items: 0}
	emptyRuns := make([]atomic.Int64, 3)
	var wg sync.WaitGroup
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go playerTask(i, npc, emptyRuns, &wg)
	}

	allDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(allDone)
	}()

	reader := bufio.NewReader(os.Stdin)
	for {
		select {
		case <-allDone:
			fmt.Println("[结束] 所有玩家都拿到了宝物。")
			return
		default:
		}

		fmt.Print("busy_wait> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Println("[结束] 输入流结束，演示退出。")
				return
			}
			fmt.Printf("[错误] 读取命令失败: %v\n", err)
			return
		}

		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "status":
			printStatus(npc, emptyRuns)
		case "restock":
			if len(fields) != 2 {
				fmt.Println("[提示] 用法: restock 3")
				continue
			}
			amount, err := strconv.Atoi(fields[1])
			if err != nil || amount <= 0 {
				fmt.Println("[提示] 补货数量必须是正整数")
				continue
			}
			applyRestock(npc, amount)
		case "quit":
			fmt.Println("[结束] 演示被手动终止。")
			return
		default:
			printHelp()
		}
	}
}
