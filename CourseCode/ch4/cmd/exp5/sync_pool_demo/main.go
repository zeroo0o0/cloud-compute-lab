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

type requestJob struct {
	id int
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
	/*
		================ 【学生重点 实验五：对象池准备】 ================
		sync.Pool 里放的是可以复用的临时 bytes.Buffer。
		本实验不是改变业务计算，也不是改变请求数量，而是只改变“临时缓冲从哪里来”。

		before 模式：每个请求都 new(bytes.Buffer)。
		after 模式：从 bufferPool.Get() 取，用完再 Put() 回来。
		============================================================
	*/
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

	jobs := make(chan requestJob, concurrency)
	var workerWG sync.WaitGroup
	/*
		================ 【学生重点 实验五：固定并发度】 ================
		这里没有一次性启动 numRequests 个 goroutine。
		而是只启动 concurrency 个 worker，让它们从 jobs 里一条条取请求。

		这样 before 和 after 的并发压力一致，主要差别就剩下“是否使用对象池”。
		============================================================
	*/
	for worker := 0; worker < concurrency; worker++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			const latencyBatchSize = 32
			batchIDs := make([]int, 0, latencyBatchSize)
			var batchStart time.Time

			flushBatch := func() {
				if len(batchIDs) == 0 {
					return
				}
				/*
					================ 【学生重点 实验五：延迟统计口径】 ================
					这里不再对“单个请求”直接 time.Now()/Since。
					原因是 after 模式单次处理太短，在部分机器上会被量成 0ns。

					现在改成：
					1. 一个 worker 连续处理一小批请求。
					2. 统计这一整批总耗时。
					3. 再折算成“平均单请求延迟”写回去。

					这样不会改变 before / after 的业务逻辑，
					但能避开计时精度太粗导致的“分位数全是 0”问题。
					==============================================================
				*/
				avgLatency := time.Since(batchStart) / time.Duration(len(batchIDs))
				for _, id := range batchIDs {
					latencies[id] = avgLatency
				}
				batchIDs = batchIDs[:0]
			}

			for job := range jobs {
				if len(batchIDs) == 0 {
					batchStart = time.Now()
				}

				var buf *bytes.Buffer
				if usePool {
					/*
						================ 【学生重点 实验五：优化后 Get】 ================
						after 模式走这里：
						1. Get：从对象池取一个旧 Buffer。
						2. Reset：清掉上次请求留下的内容，但尽量复用底层内存。

						这就是降低 malloc 和 GC 压力的关键写法。
						==============================================================
					*/
					buf = bufferPool.Get().(*bytes.Buffer)
					buf.Reset()
				} else {
					/*
						================ 【学生重点 实验五：优化前 new】 ================
						before 模式走这里：
						每个请求都重新创建一个 bytes.Buffer，并为它准备 payloadSize 大小的空间。

						请求次数很多时，这些临时对象会带来更多内存分配和 GC 压力。
						==============================================================
					*/
					buf = new(bytes.Buffer)
					buf.Grow(payloadSize)
				}

				buf.Write(mockData)
				resultSink[job.id] = len(buf.Bytes()) + doSomeWork(workIterations)

				if usePool {
					/*
						================ 【学生重点 实验五：优化后 Put】 ================
						请求处理完以后，把 Buffer 放回对象池。
						下一次请求再 Get 时，就可能复用这块已经申请过的内存。
						==============================================================
					*/
					bufferPool.Put(buf)
				}
				batchIDs = append(batchIDs, job.id)
				if len(batchIDs) == latencyBatchSize {
					flushBatch()
				}
			}

			flushBatch()
		}()
	}

	for i := 0; i < numRequests; i++ {
		jobs <- requestJob{
			id: i,
		}
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
	fmt.Println("延迟口径: 按小批次计时，再折算平均单请求延迟，避免单次计时精度过粗。")

	s := runExperiment(usePool, *numRequests, *payloadKB*1024, *workIterations, *concurrency)
	if usePool {
		printStats("优化后：+sync.Pool", s)
		fmt.Println("\n[提示] 请与 before 模式在相同参数下的 Mallocs、GC 和 P99.9 对比。")
	} else {
		printStats("优化前：频繁分配临时对象", s)
		fmt.Println("\n[提示] 现在请用相同参数运行 after 模式，再对比两次结果。")
	}
}
