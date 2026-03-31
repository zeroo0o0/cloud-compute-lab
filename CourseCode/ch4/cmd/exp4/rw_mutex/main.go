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

func formatMS(d time.Duration) string {
	return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000.0)
}

func Reader(name string, hold time.Duration, base time.Time, wg *sync.WaitGroup) {
	defer wg.Done()

	arrive := time.Now()
	fmt.Printf("[%s][读者 %s] 尝试获取读锁\n", formatMS(time.Since(base)), name)
	rw.RLock()
	fmt.Printf("[%s][读者 %s] 获取读锁成功，等待了 %s，读取数据: %d\n",
		formatMS(time.Since(base)), name, formatMS(time.Since(arrive)), data["key"])
	time.Sleep(hold)
	fmt.Printf("[%s][读者 %s] 释放读锁\n", formatMS(time.Since(base)), name)
	rw.RUnlock()
}

func Writer(name string, val int, hold time.Duration, base time.Time, wg *sync.WaitGroup) {
	defer wg.Done()

	arrive := time.Now()
	fmt.Printf("[%s][写者 %s] 尝试获取写锁\n", formatMS(time.Since(base)), name)
	rw.Lock()
	fmt.Printf("[%s][写者 %s] 获取写锁成功，等待了 %s，开始写入数据: %d\n",
		formatMS(time.Since(base)), name, formatMS(time.Since(arrive)), val)
	time.Sleep(hold)
	data["key"] = val
	fmt.Printf("[%s][写者 %s] 写入完成，准备释放写锁\n", formatMS(time.Since(base)), name)
	rw.Unlock()
}

func main() {
	fmt.Println("=== 实验四：锁的进阶技巧与粒度优化 / RWMutex ===")
	fmt.Println("目标: 观察读者并发、写者独占，以及“写者排队后/写入期间到来的读者”都会被阻塞。")
	var wg sync.WaitGroup
	base := time.Now()

	wg.Add(1)
	go Reader("R1", 1200*time.Millisecond, base, &wg)

	wg.Add(1)
	go Reader("R2", 1200*time.Millisecond, base, &wg)

	time.Sleep(100 * time.Millisecond)
	wg.Add(1)
	go Writer("W1", 999, 1*time.Second, base, &wg)

	time.Sleep(150 * time.Millisecond)
	wg.Add(1)
	go Reader("R3(写者排队后到达)", 400*time.Millisecond, base, &wg)

	time.Sleep(1500 * time.Millisecond)
	wg.Add(1)
	go Reader("R4(写入进行中到达)", 400*time.Millisecond, base, &wg)

	wg.Wait()
	fmt.Println("[提示] 重点看 R3 和 R4 的“尝试获取读锁”与“真正拿到读锁”之间的等待时间。")
}
