package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	target := flag.String("target", "http://localhost:8080", "Game service URL")
	concurrency := flag.Int("c", 10, "Concurrent workers")
	duration := flag.Duration("d", 60*time.Second, "Test duration")
	flag.Parse()

	fmt.Printf("=== Load Test ===\n")
	fmt.Printf("Target:      %s\n", *target)
	fmt.Printf("Concurrency: %d\n", *concurrency)
	fmt.Printf("Duration:    %v\n", *duration)
	fmt.Println()

	var totalRequests int64
	var successRequests int64
	var failedRequests int64

	ctx := make(chan struct{})
	go func() {
		time.Sleep(*duration)
		close(ctx)
	}()

	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{Timeout: 10 * time.Second}
			for {
				select {
				case <-ctx:
					return
				default:
					atomic.AddInt64(&totalRequests, 1)
					resp, err := client.Get(*target + "/move?player=loadtest&dir=north")
					if err != nil {
						atomic.AddInt64(&failedRequests, 1)
						time.Sleep(100 * time.Millisecond)
						continue
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.StatusCode == 200 {
						atomic.AddInt64(&successRequests, 1)
					} else {
						atomic.AddInt64(&failedRequests, 1)
					}
				}
			}
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			printSummary(totalRequests, successRequests, failedRequests, *duration)
			os.Exit(0)
		case <-ticker.C:
			fmt.Printf("[%s] Total: %d | OK: %d | Fail: %d\n",
				time.Now().Format("15:04:05"),
				atomic.LoadInt64(&totalRequests),
				atomic.LoadInt64(&successRequests),
				atomic.LoadInt64(&failedRequests))
		}
	}
}

func printSummary(total, ok, fail int64, dur time.Duration) {
	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Printf("Total Requests:  %d\n", total)
	fmt.Printf("Successful:      %d\n", ok)
	fmt.Printf("Failed:          %d\n", fail)
	fmt.Printf("RPS:             %.1f\n", float64(total)/dur.Seconds())
	fmt.Printf("Success Rate:    %.1f%%\n", float64(ok)/float64(total)*100)
}
