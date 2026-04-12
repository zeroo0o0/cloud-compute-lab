package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"
)

func doSomeWork(iterations int) int {
	sum := 0
	for j := 0; j < iterations; j++ {
		sum += j
	}
	return sum
}

type stats struct {
	totalDuration time.Duration
	allocs        uint64
	gcCount       uint32
	latencies     []time.Duration
}

func runExperiment(
	usePool bool,
	numRequests int,
	payloadSize int,
	workIterations int,
	concurrency int,
) stats {
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > numRequests {
		concurrency = numRequests
	}

	mockData := bytes.Repeat([]byte("A"), payloadSize)
	bufferPool := sync.Pool{
		New: func() any {
			buf := new(bytes.Buffer)
			buf.Grow(payloadSize)
			return buf
		},
	}

	latencies := make([]time.Duration, numRequests)
	resultSink := make([]int, numRequests)

	runtime.GC()
	var startMem runtime.MemStats
	runtime.ReadMemStats(&startMem)
	start := time.Now()

	jobs := make(chan int, concurrency)
	var workerWG sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for id := range jobs {
				begin := time.Now()

				var buf *bytes.Buffer
				if usePool {
					buf = bufferPool.Get().(*bytes.Buffer)
					buf.Reset()
				} else {
					buf = new(bytes.Buffer)
					buf.Grow(payloadSize)
				}

				buf.Write(mockData)
				resultSink[id] = len(buf.Bytes()) + doSomeWork(workIterations)

				if usePool {
					bufferPool.Put(buf)
				}
				latencies[id] = time.Since(begin)
			}
		}()
	}

	for i := 0; i < numRequests; i++ {
		jobs <- i
	}
	close(jobs)
	workerWG.Wait()
	runtime.KeepAlive(resultSink)

	var endMem runtime.MemStats
	runtime.ReadMemStats(&endMem)

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	return stats{
		totalDuration: time.Since(start),
		allocs:        endMem.Mallocs - startMem.Mallocs,
		gcCount:       endMem.NumGC - startMem.NumGC,
		latencies:     latencies,
	}
}

func printStats(label string, s stats) {
	length := len(s.latencies)
	p50 := s.latencies[length*50/100]
	p90 := s.latencies[length*90/100]
	p99 := s.latencies[length*99/100]
	p999 := s.latencies[length*999/1000]

	fmt.Printf("\n[%s]\n", label)
	fmt.Printf("总耗时: %v\n", s.totalDuration)
	fmt.Printf("堆内存分配 (Mallocs): %d 次\n", s.allocs)
	fmt.Printf("触发 GC 次数: %d 次\n", s.gcCount)
	fmt.Println("------------- 延迟分布 -------------")
	fmt.Printf("P50   : %.3f 微秒\n", float64(p50.Nanoseconds())/1000.0)
	fmt.Printf("P90   : %.3f 微秒\n", float64(p90.Nanoseconds())/1000.0)
	fmt.Printf("P99   : %.3f 微秒\n", float64(p99.Nanoseconds())/1000.0)
	fmt.Printf("P99.9 : %.3f 微秒\n", float64(p999.Nanoseconds())/1000.0)
	fmt.Println("-----------------------------------")
}

func usage() {
	fmt.Println("用法:")
	fmt.Println("  go run ./cmd/exp5/sync_pool_demo before [-requests 12000] [-payload-kb 10] [-work 500] [-concurrency 32]")
	fmt.Println("  go run ./cmd/exp5/sync_pool_demo after  [-requests 12000] [-payload-kb 10] [-work 500] [-concurrency 32]")
	fmt.Println()
	fmt.Println("建议先运行 before，再用完全相同的参数运行 after。")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	mode := os.Args[1]
	fs := flag.NewFlagSet(mode, flag.ExitOnError)
	numRequests := fs.Int("requests", 12000, "请求总数")
	payloadKB := fs.Int("payload-kb", 10, "每次请求的临时缓冲大小（KB）")
	workIterations := fs.Int("work", 500, "每次请求的额外计算量")
	concurrency := fs.Int("concurrency", 32, "固定并发度，用于更严格地控制变量")
	fs.Parse(os.Args[2:])

	if mode != "before" && mode != "after" {
		usage()
		return
	}
	if *numRequests < 1 {
		fmt.Println("requests 必须大于 0")
		return
	}

	usePool := mode == "after"
	fmt.Println("=== 实验五：高并发性能榨取（sync.Pool） ===")
	if usePool {
		fmt.Println("当前模式: 优化后（使用 sync.Pool 复用临时缓冲）")
	} else {
		fmt.Println("当前模式: 优化前（每次都 new 一个临时缓冲）")
	}
	fmt.Printf("本次参数: requests=%d, payload=%dKB, work=%d, concurrency=%d\n",
		*numRequests, *payloadKB, *workIterations, *concurrency)
	fmt.Println("说明: 把“是否使用对象池”作为核心对照变量，其他参数保持一致。")

	s := runExperiment(usePool, *numRequests, *payloadKB*1024, *workIterations, *concurrency)
	if usePool {
		printStats("优化后：+sync.Pool", s)
		fmt.Println("\n[提示] 请与 before 模式在相同参数下的 Mallocs、GC 和 P99.9 对比。")
	} else {
		printStats("优化前：频繁分配临时对象", s)
		fmt.Println("\n[提示] 现在请用相同参数运行 after 模式，再对比两次结果。")
	}
}
