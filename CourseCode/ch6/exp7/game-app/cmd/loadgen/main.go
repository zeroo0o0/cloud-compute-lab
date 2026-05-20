package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

func main() {
	// 解析命令行参数：压测目标地址、每级压力持续时间
	target := flag.String("url", "http://game-service:8081/burn?ms=250", "target URL")
	stageDuration := flag.Duration("stage-duration", 4*time.Second, "duration of each load stage")
	flag.Parse()
	client := &http.Client{Timeout: 5 * time.Second}
	// 压力阶梯：并发数从2→4→8→16 逐级增加
	stages := []int{2, 4, 8, 16}
	log.Printf("[loadgen] target=%s stages=%v stageDuration=%s", *target, stages, *stageDuration)

	var wg sync.WaitGroup

	// 函数：批量启动压测协程（worker）
	startWorkers := func(from, to int) {
		for i := from; i < to; i++ {
			wg.Add(1)
			// 启动一个协程，无限发送压测请求
			go func(id int) {
				defer wg.Done()
				// 死循环：持续压测，永不停止
				for {
					// 向game服务发送请求，消耗CPU
					resp, err := client.Get(*target)
					if err != nil {
						log.Printf("[loadgen] worker=%d request failed: %v", id, err)
						time.Sleep(200 * time.Millisecond)
						continue
					}
					// 忽略响应体，仅完成请求
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
			}(i)
		}
	}

	activeWorkers := 0
	// 核心：逐级加压，每级等待4秒后升级并发数
	for _, targetWorkers := range stages {
		startWorkers(activeWorkers, targetWorkers)
		activeWorkers = targetWorkers
		log.Printf("[loadgen] active workers=%d", activeWorkers)
		// 维持当前压力，等待4秒
		time.Sleep(*stageDuration)
	}

	// 最终保持16个并发，永久压测
	log.Printf("[loadgen] holding final stage at workers=%d", activeWorkers)
	wg.Wait()
}
