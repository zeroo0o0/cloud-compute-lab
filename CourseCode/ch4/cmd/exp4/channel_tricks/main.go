package main

import (
	"fmt"
	"sync"
	"time"
)

func demoSemaphore() {
	fmt.Println("\n>>> 子演示 A：利用 Channel 实现容量为 3 的信号量 <<<")
	semaphore := make(chan struct{}, 3)
	var wg sync.WaitGroup

	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			semaphore <- struct{}{}
			fmt.Printf("[Worker %d] 获取许可，正在执行...\n", id)
			time.Sleep(500 * time.Millisecond)

			fmt.Printf("[Worker %d]工作完成，执行完毕，释放许可\n", id)
			<-semaphore
		}(i)
	}
	wg.Wait()
	fmt.Println("信号量测试结束。\n")
}

func demoTimeoutLock() {
	fmt.Println(">>> 子演示 B：带超时的锁（获取失败后走降级逻辑） <<<")
	lock := make(chan struct{}, 1)

	go func() {
		lock <- struct{}{}
		fmt.Println("[玩家A] 抢到了锁，违规占用 5 秒钟")
		time.Sleep(5 * time.Second)
		<-lock
	}()

	time.Sleep(100 * time.Millisecond)
	fmt.Println("[玩家B] 尝试获取锁，设置超时时间为 2 秒...")

	select {
	case lock <- struct{}{}:
		fmt.Println("[玩家B] 成功获取到锁！")
		<-lock
	case <-time.After(2 * time.Second):
		fmt.Println("[玩家B] 获取锁超时，执行降级逻辑 (doFallback)，避免卡死。")
	}
}

func main() {
	fmt.Println("=== 实验四：锁的进阶技巧与粒度优化 / Channel 技巧 ===")
	fmt.Println("目标: 展示信号量限流和超时降级两种常用工程技巧。")
	demoSemaphore()
	time.Sleep(1 * time.Second)
	demoTimeoutLock()
	fmt.Println("\n[结论] Channel 不只是通信工具，也能自然表达“许可数”和“带超时的资源竞争”。")
}
