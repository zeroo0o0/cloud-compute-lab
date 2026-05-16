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
	target := flag.String("url", "http://game-service:8081/burn?ms=250", "target URL")
	stageDuration := flag.Duration("stage-duration", 4*time.Second, "duration of each load stage")
	flag.Parse()

	client := &http.Client{Timeout: 5 * time.Second}
	stages := []int{2, 4, 8, 16}
	log.Printf("[loadgen] target=%s stages=%v stageDuration=%s", *target, stages, *stageDuration)

	var wg sync.WaitGroup
	startWorkers := func(from, to int) {
		for i := from; i < to; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for {
					resp, err := client.Get(*target)
					if err != nil {
						log.Printf("[loadgen] worker=%d request failed: %v", id, err)
						time.Sleep(200 * time.Millisecond)
						continue
					}
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
					if resp.StatusCode >= http.StatusBadRequest {
						log.Printf("[loadgen] worker=%d status=%d", id, resp.StatusCode)
					}
				}
			}(i)
		}
	}

	activeWorkers := 0
	for _, targetWorkers := range stages {
		startWorkers(activeWorkers, targetWorkers)
		activeWorkers = targetWorkers
		log.Printf("[loadgen] active workers=%d", activeWorkers)
		time.Sleep(*stageDuration)
	}

	log.Printf("[loadgen] holding final stage at workers=%d", activeWorkers)
	wg.Wait()
}
