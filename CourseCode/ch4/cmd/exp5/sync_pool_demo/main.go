package main

import (
	"bytes"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"
)

var mockData = bytes.Repeat([]byte("A"), 10*1024)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

var dummy int

//go:noinline
func doSomeWork() {
	for j := 0; j < 500; j++ {
		dummy += j
	}
}

func processWithoutPool(id int, latencies []time.Duration, wg *sync.WaitGroup) {
	defer wg.Done()
	start := time.Now()

	buf := new(bytes.Buffer)
	buf.Grow(10 * 1024)
	buf.Write(mockData)
	_ = buf.Bytes()

	doSomeWork()
	latencies[id] = time.Since(start)
}

func processWithPool(id int, latencies []time.Duration, wg *sync.WaitGroup) {
	defer wg.Done()
	start := time.Now()

	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Write(mockData)
	_ = buf.Bytes()

	doSomeWork()
	bufferPool.Put(buf)
	latencies[id] = time.Since(start)
}

func printPercentiles(name string, latencies []time.Duration, startMem runtime.MemStats, startTime time.Time) {
	var endMem runtime.MemStats
	runtime.ReadMemStats(&endMem)

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	totalDuration := time.Since(startTime)
	allocs := endMem.Mallocs - startMem.Mallocs
	gcCount := endMem.NumGC - startMem.NumGC

	length := len(latencies)
	p50 := latencies[length*50/100]
	p90 := latencies[length*90/100]
	p99 := latencies[length*99/100]
	p999 := latencies[length*999/1000]

	fmt.Printf("\n[%s]\n", name)
	fmt.Printf("总耗时 (涵盖调度): %v\n", totalDuration)
	fmt.Printf("堆内存分配 (Mallocs): %d 次\n", allocs)
	fmt.Printf("触发 GC 次数: %d 次\n", gcCount)
	fmt.Println("------------- 延迟分布 (微秒/毫秒) -------------")
	fmt.Printf("P50  (中位数): %.3f 微秒\n", float64(p50.Nanoseconds())/1000.0)
	fmt.Printf("P90  延迟    : %.3f 微秒\n", float64(p90.Nanoseconds())/1000.0)
	fmt.Printf("P99  延迟    : %.3f 微秒\n", float64(p99.Nanoseconds())/1000.0)
	fmt.Printf("P99.9延迟    : %.3f 微秒  <-- 观察对比项\n", float64(p999.Nanoseconds())/1000.0)
	fmt.Println("------------------------------------------------")
}

func main() {
	const numRequests = 20000
	var wg sync.WaitGroup

	fmt.Println("=== 实验五：高并发性能榨取（sync.Pool） ===")
	fmt.Println("场景: 高频请求里频繁申请 10KB 临时缓冲，对比“每次 new”与“对象复用”。")
	fmt.Println("说明: 本实验只做本地对象池演示，数据库连接池保留为概念扩展。")

	latenciesNoPool := make([]time.Duration, numRequests)
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	t1 := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go processWithoutPool(i, latenciesNoPool, &wg)
	}
	wg.Wait()
	printPercentiles("基准：无优化 (频繁 10KB 分配)", latenciesNoPool, m1, t1)

	time.Sleep(1 * time.Second)

	latenciesWithPool := make([]time.Duration, numRequests)
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	t2 := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go processWithPool(i, latenciesWithPool, &wg)
	}
	wg.Wait()
	printPercentiles("优化：+sync.Pool (复用 10KB 缓冲)", latenciesWithPool, m2, t2)

	fmt.Println("\n[提示] 重点对比两组结果里的 Mallocs、GC 次数和 P99.9 延迟。")
	fmt.Println("[扩展] 真正的数据库连接池属于另一类资源池，本章只做对象池演示。")
}
