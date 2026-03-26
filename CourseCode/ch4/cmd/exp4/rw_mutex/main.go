package main

import (
	"fmt"
	"sync"
	"time"
)

var (
	data = map[string]int{"key": 100}
	rw   sync.RWMutex
)

func Reader(id int, wg *sync.WaitGroup) {
	defer wg.Done()

	rw.RLock()
	fmt.Printf("[读者 %d]  进入阅览室，读取数据: %d\n", id, data["key"])
	time.Sleep(1 * time.Second)
	fmt.Printf("[读者 %d] 退出阅览室\n", id)
	rw.RUnlock()
}

func Writer(val int, wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Printf("【写者】  尝试获取写锁，等待其他人离开...\n")
	rw.Lock()
	fmt.Printf("【写者】  已清场！独占阅览室，开始写入数据: %d\n", val)
	time.Sleep(2 * time.Second)
	data["key"] = val
	fmt.Printf("【写者】  写入完成，开放阅览室\n")
	rw.Unlock()
}

func main() {
	fmt.Println("=== 实验四：锁的进阶技巧与粒度优化 / RWMutex ===")
	fmt.Println("目标: 观察“读多写少”场景下，多个读者并发进入，而写者仍保持独占。")
	var wg sync.WaitGroup

	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go Reader(i, &wg)
	}

	time.Sleep(200 * time.Millisecond)
	wg.Add(1)
	go Writer(999, &wg)

	time.Sleep(200 * time.Millisecond)
	for i := 4; i <= 5; i++ {
		wg.Add(1)
		go Reader(i, &wg)
	}

	wg.Wait()
	fmt.Println("[结论] RWMutex 让“读”具备并发性，但只要有写者排队，后续读者也必须让路。")
}
