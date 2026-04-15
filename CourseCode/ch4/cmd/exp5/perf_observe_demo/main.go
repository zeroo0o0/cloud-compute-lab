package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"sync"
	"time"
)

func startLeakDemo(stop <-chan struct{}) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			blockForever := make(chan struct{})
			go func() {
				/*
					================ 【学生重点 实验五：Goroutine 反例】 ================
					这里故意创建一个永远收不到信号的 goroutine。
					持续运行时，goroutine profile 里会看到协程数量一路上涨。
					===============================================================
				*/
				<-blockForever
			}()
		}
	}
}

func startFixedDemo(stop <-chan struct{}) {
	var wg sync.WaitGroup
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			wg.Wait()
			return
		case <-ticker.C:
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case <-time.After(50 * time.Millisecond):
				case <-stop:
				}
			}()
		}
	}
}

func main() {
	mode := flag.String("mode", "leak", "运行模式：leak 或 fixed")
	addr := flag.String("addr", "127.0.0.1:6060", "pprof HTTP 地址")
	seconds := flag.Int("seconds", 20, "演示持续秒数")
	flag.Parse()

	stop := make(chan struct{})
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Printf("[pprof] 监听 %s 失败: %v\n", *addr, err)
		fmt.Println("[pprof] 如果 6060 被占用，可改用: go run ./cmd/exp5/perf_observe_demo -mode leak -seconds 60 -addr 127.0.0.1:6061")
		return
	}
	defer ln.Close()
	go func() {
		if err := http.Serve(ln, nil); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[pprof] HTTP server stopped: %v\n", err)
		}
	}()

	fmt.Println("=== 实验五：goroutine 性能观测演示 ===")
	fmt.Printf("mode=%s, pprof=http://%s/debug/pprof/, duration=%ds\n", *mode, *addr, *seconds)
	fmt.Printf("[pprof] HTTP 服务已启动，正在监听 %s\n", *addr)
	fmt.Println("建议另开一个终端执行：")
	fmt.Printf("  go tool pprof -top http://%s/debug/pprof/goroutine\n", *addr)
	fmt.Printf("  PowerShell: (Invoke-WebRequest http://%s/debug/pprof/goroutine?debug=1).Content\n\n", *addr)

	switch *mode {
	case "leak":
		fmt.Println("当前模式：反例。协程会越来越多。")
		go startLeakDemo(stop)
	case "fixed":
		fmt.Println("当前模式：修复版。协程会及时退出。")
		go startFixedDemo(stop)
	default:
		fmt.Println("mode 只能是 leak 或 fixed")
		return
	}

	deadline := time.Now().Add(time.Duration(*seconds) * time.Second)
	for time.Now().Before(deadline) {
		fmt.Printf("[状态] goroutines=%d\n", runtime.NumGoroutine())
		time.Sleep(2 * time.Second)
	}

	close(stop)
	time.Sleep(300 * time.Millisecond)
	fmt.Printf("[结束] goroutines=%d\n", runtime.NumGoroutine())
}
