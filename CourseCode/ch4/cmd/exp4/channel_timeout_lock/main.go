package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("=== 实验四：锁的进阶技巧与粒度优化 / Channel 超时锁 ===")
	fmt.Println("目标: 用 select + time.After 模拟带超时的资源竞争与降级逻辑。")

	lock := make(chan struct{}, 1)

	go func() {
		lock <- struct{}{}
		fmt.Println("[玩家A] 抢到了资源，故意占用 5 秒")
		time.Sleep(5 * time.Second)
		fmt.Println("[玩家A] 处理完成，释放资源")
		<-lock
	}()

	time.Sleep(100 * time.Millisecond)
	fmt.Println("[玩家B] 尝试获取资源，超时时间设置为 2 秒")

	/*
		================ 【学生重点 实验四：带超时的锁】 ================
		请盯住这个 select：
		1. case lock <- struct{}{}：如果资源空出来，玩家B拿到锁并继续处理。
		2. case <-time.After(2 * time.Second)：如果等了 2 秒还拿不到，就走降级逻辑。

		这段代码对应实验四的“不要让玩家永远卡死在抢锁上”。
		============================================================
	*/
	select {
	case lock <- struct{}{}:
		fmt.Println("[玩家B] 成功获取资源")
		<-lock
	case <-time.After(2 * time.Second):
		fmt.Println("[玩家B] 等待超时，转入降级逻辑，避免一直卡住")
	}

	fmt.Println("[提示] 如果去掉超时分支，玩家B 会一直堵在这里。")
}
