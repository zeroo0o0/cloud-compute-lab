package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

type readWriteLocker interface {
	Lock()
	Unlock()
	RLock()
	RUnlock()
	Name() string
}

type mutexAdapter struct {
	mu sync.Mutex
}

func (m *mutexAdapter) Lock()    { m.mu.Lock() }
func (m *mutexAdapter) Unlock()  { m.mu.Unlock() }
func (m *mutexAdapter) RLock()   { m.mu.Lock() }
func (m *mutexAdapter) RUnlock() { m.mu.Unlock() }
func (m *mutexAdapter) Name() string {
	return "普通 Mutex"
}

type rwMutexAdapter struct {
	mu sync.RWMutex
}

func (m *rwMutexAdapter) Lock()    { m.mu.Lock() }
func (m *rwMutexAdapter) Unlock()  { m.mu.Unlock() }
func (m *rwMutexAdapter) RLock()   { m.mu.RLock() }
func (m *rwMutexAdapter) RUnlock() { m.mu.RUnlock() }
func (m *rwMutexAdapter) Name() string {
	return "RWMutex"
}

type metric struct {
	Name    string
	Kind    string
	Wait    time.Duration
	Started time.Duration
}

type scenarioResult struct {
	Name            string
	TotalDuration   time.Duration
	ReaderWaitTotal time.Duration
	MaxReaderWait   time.Duration
	WriterWait      time.Duration
	Metrics         []metric
}

func formatMS(d time.Duration) string {
	return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000.0)
}

func runScenario(locker readWriteLocker) scenarioResult {
	value := 100
	base := time.Now()
	metricsCh := make(chan metric, 5)
	var wg sync.WaitGroup

	reader := func(name string, startAfter, hold time.Duration) {
		defer wg.Done()
		time.Sleep(startAfter)

		arrive := time.Now()
		fmt.Printf("[%s][%s] 尝试获取读锁\n", formatMS(time.Since(base)), name)
		locker.RLock()
		waited := time.Since(arrive)
		started := time.Since(base)
		fmt.Printf("[%s][%s] 获取读锁成功，等待=%s，读取数据=%d\n", formatMS(started), name, formatMS(waited), value)
		time.Sleep(hold)
		fmt.Printf("[%s][%s] 释放读锁\n", formatMS(time.Since(base)), name)
		locker.RUnlock()
		metricsCh <- metric{Name: name, Kind: "reader", Wait: waited, Started: started}
	}

	writer := func(name string, newValue int, startAfter, hold time.Duration) {
		defer wg.Done()
		time.Sleep(startAfter)

		arrive := time.Now()
		fmt.Printf("[%s][%s] 尝试获取写锁\n", formatMS(time.Since(base)), name)
		locker.Lock()
		waited := time.Since(arrive)
		started := time.Since(base)
		fmt.Printf("[%s][%s] 获取写锁成功，等待=%s，准备写入=%d\n", formatMS(started), name, formatMS(waited), newValue)
		time.Sleep(hold)
		value = newValue
		fmt.Printf("[%s][%s] 写入完成，释放写锁\n", formatMS(time.Since(base)), name)
		locker.Unlock()
		metricsCh <- metric{Name: name, Kind: "writer", Wait: waited, Started: started}
	}

	wg.Add(5)
	go reader("R1(首批读者)", 0, 1200*time.Millisecond)
	go reader("R2(首批读者)", 0, 1200*time.Millisecond)
	go writer("W1(写者)", 999, 100*time.Millisecond, 1*time.Second)
	go reader("R3(写者排队后到达)", 250*time.Millisecond, 400*time.Millisecond)
	go reader("R4(写入进行中到达)", 1500*time.Millisecond, 400*time.Millisecond)

	wg.Wait()
	close(metricsCh)

	result := scenarioResult{Name: locker.Name(), TotalDuration: time.Since(base)}
	for item := range metricsCh {
		result.Metrics = append(result.Metrics, item)
		if item.Kind == "reader" {
			result.ReaderWaitTotal += item.Wait
			if item.Wait > result.MaxReaderWait {
				result.MaxReaderWait = item.Wait
			}
			continue
		}
		result.WriterWait = item.Wait
	}
	sort.Slice(result.Metrics, func(i, j int) bool {
		return result.Metrics[i].Started < result.Metrics[j].Started
	})
	return result
}

func printScenarioSummary(result scenarioResult) {
	fmt.Printf("\n[%s 摘要]\n", result.Name)
	for _, item := range result.Metrics {
		fmt.Printf("- %s (%s): 开始进入临界区=%s, 等待=%s\n", item.Name, item.Kind, formatMS(item.Started), formatMS(item.Wait))
	}
	fmt.Printf("总耗时: %s\n", formatMS(result.TotalDuration))
	fmt.Printf("读者累计等待: %s\n", formatMS(result.ReaderWaitTotal))
	fmt.Printf("最长读者等待: %s\n", formatMS(result.MaxReaderWait))
	fmt.Printf("写者等待: %s\n", formatMS(result.WriterWait))
}

func printComparison(before, after scenarioResult) {
	fmt.Println("\n========== 对比结果 ==========")
	fmt.Printf("无读写锁(%s): 总耗时=%s, 读者累计等待=%s, 最长读者等待=%s\n",
		before.Name, formatMS(before.TotalDuration), formatMS(before.ReaderWaitTotal), formatMS(before.MaxReaderWait))
	fmt.Printf("使用读写锁(%s): 总耗时=%s, 读者累计等待=%s, 最长读者等待=%s\n",
		after.Name, formatMS(after.TotalDuration), formatMS(after.ReaderWaitTotal), formatMS(after.MaxReaderWait))
	fmt.Println("结论: 在读多写少场景里，RWMutex 让首批读者并发读取；写者一旦排队，后续读者仍会被拦住。")
	fmt.Println("==============================")
}

func waitForEnter(reader *bufio.Reader, prompt string) {
	fmt.Println(prompt)
	_, _ = reader.ReadString('\n')
}

func main() {
	fmt.Println("=== 实验四：锁的进阶技巧与粒度优化 / RWMutex 对照实验 ===")
	fmt.Println("目标: 先看“没有读写锁”时的串行效果，再暂停，随后用同一组操作演示 RWMutex。")

	reader := bufio.NewReader(os.Stdin)
	waitForEnter(reader, "[操作] 按回车运行第 1 组：普通 Mutex 对照。")
	mutexResult := runScenario(&mutexAdapter{})
	printScenarioSummary(mutexResult)

	waitForEnter(reader, "\n[操作] 按回车运行第 2 组：RWMutex 对照。")
	rwResult := runScenario(&rwMutexAdapter{})
	printScenarioSummary(rwResult)
	printComparison(mutexResult, rwResult)
}
