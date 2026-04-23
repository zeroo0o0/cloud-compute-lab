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
	cond  *sync.Cond
}

func playerGetTreasure(playerID int, npc *NPC, waiting []atomic.Bool, wakeups []atomic.Int64, wg *sync.WaitGroup) {
	defer wg.Done()

	npc.mu.Lock()
	/*
		================ 【学生重点 实验三：Cond 等待】 ================
		请把下面这段和 busy_wait 版本对照看：
		1. for npc.Items == 0：库存为空时不要继续往下拿。
		2. npc.cond.Wait()：释放锁，并让当前玩家睡眠等待。
		3. 被 Signal 唤醒后，重新回到 for 条件再检查一次库存。

		这里必须用 for，而不是 if。
		因为被叫醒只代表“可能有变化”，不代表自己一定能拿到宝物。
		============================================================
	*/
	for npc.Items == 0 {
		waiting[playerID-1].Store(true)
		fmt.Printf("[等待] 玩家 %d 进入等待队列，释放锁并睡眠。\n", playerID)
		npc.cond.Wait()
		waiting[playerID-1].Store(false)
		wakeups[playerID-1].Add(1)
		fmt.Printf("[唤醒] 玩家 %d 被叫醒，重新检查库存。\n", playerID)
	}

	npc.Items--
	left := npc.Items
	npc.mu.Unlock()
	fmt.Printf("[成功] 玩家 %d 取走宝物！剩余宝物数: %d\n", playerID, left)
}

func restock(npc *NPC, amount int) {
	npc.mu.Lock()
	npc.Items += amount
	current := npc.Items
	npc.mu.Unlock()

	fmt.Printf("[系统] NPC 补货 %d 个宝物，当前库存=%d\n", amount, current)
	/*
		================ 【学生重点 实验三：Signal 唤醒】 ================
		补货以后，NPC 不再要求玩家自己反复轮询库存。
		这里每调用一次 Signal，就叫醒一个正在 Wait 的玩家，让他回去重新检查库存。

		这就是实验三从“忙等消耗 CPU”改成“有货再通知”的关键写法。
		==============================================================
	*/
	for i := 0; i < amount; i++ {
		npc.cond.Signal()
	}
}

func printStatus(npc *NPC, waiting []atomic.Bool, wakeups []atomic.Int64) {
	npc.mu.Lock()
	items := npc.Items
	npc.mu.Unlock()

	fmt.Println("------ 当前状态（Cond 版） ------")
	fmt.Printf("NPC 库存: %d\n", items)
	for idx := range waiting {
		state := "运行中"
		if waiting[idx].Load() {
			state = "睡眠等待中"
		}
		fmt.Printf("玩家 %d 状态: %s, 被唤醒次数: %d\n", idx+1, state, wakeups[idx].Load())
	}
	fmt.Println("-------------------------------")
}

func printHelp() {
	fmt.Println("可用命令:")
	fmt.Println("  status       查看库存、等待状态和唤醒次数")
	fmt.Println("  restock N    补货 N 个宝物，例如 restock 3")
	fmt.Println("  quit         退出演示")
}

func main() {
	fmt.Println("=== 实验三：告别忙等 / Cond 修复版 ===")
	fmt.Println("场景: 玩家在库存为空时不再轮询，而是进入等待队列。")
	fmt.Println("目标: 把“补货”交给操作者，直观看 Wait/Signal 的睡眠与唤醒。")
	printHelp()

	npc := &NPC{ID: 999, Items: 0}
	npc.cond = sync.NewCond(&npc.mu)

	waiting := make([]atomic.Bool, 3)
	wakeups := make([]atomic.Int64, 3)

	var wg sync.WaitGroup
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go playerGetTreasure(i, npc, waiting, wakeups, &wg)
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

		fmt.Print("cond_wait> ")
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
			printStatus(npc, waiting, wakeups)
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
			restock(npc, amount)
		case "quit":
			fmt.Println("[结束] 演示被手动终止。")
			return
		default:
			printHelp()
		}
	}
}
