package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	fmt.Println("=== 实验四：锁的进阶技巧与粒度优化 / Channel 信号量 ===")
	fmt.Println("目标: 使用容量为 3 的 Channel 限制同时工作的协程数量。")

	semaphore := make(chan struct{}, 3)
	var wg sync.WaitGroup

	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			fmt.Printf("[Worker %d] 尝试获取许可...\n", id)
			semaphore <- struct{}{}
			fmt.Printf("[Worker %d] 获取许可，开始执行\n", id)
			time.Sleep(500 * time.Millisecond)
			fmt.Printf("[Worker %d] 工作完成，释放许可\n", id)
			<-semaphore
		}(i)
	}

	wg.Wait()
	fmt.Println("[提示] 观察同一时刻真正进入执行区的 Worker 数量是否被限制在 3 个以内。")
}
