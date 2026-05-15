package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

var startTime = time.Now()
var cpuSpinners int32

type HealthResp struct {
	Status    string  `json:"status"`
	Uptime    string  `json:"uptime"`
	CPULoad   float64 `json:"cpu_load_percent"`
	AllocMB   float64 `json:"alloc_mb"`
	Goroutines int    `json:"goroutines"`
}

type StressResp struct {
	Workers    int   `json:"workers"`
	DurationMs int64 `json:"duration_ms"`
	Message    string `json:"message"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	resp := HealthResp{
		Status:     "ok",
		Uptime:     time.Since(startTime).Round(time.Second).String(),
		CPULoad:    float64(atomic.LoadInt32(&cpuSpinners)) / float64(runtime.NumCPU()) * 100,
		AllocMB:    float64(m.Alloc) / 1024 / 1024,
		Goroutines: runtime.NumGoroutine(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func stressHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	durationStr := r.URL.Query().Get("duration")
	if durationStr == "" {
		durationStr = "5s"
	}
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		duration = 5 * time.Second
	}
	numCPU := runtime.NumCPU()
	log.Printf("[STRESS] Starting %d CPU workers for %v", numCPU, duration)
	var active int32
	for i := 0; i < numCPU; i++ {
		atomic.AddInt32(&cpuSpinners, 1)
		atomic.AddInt32(&active, 1)
		go func() {
			defer atomic.AddInt32(&cpuSpinners, -1)
			end := time.After(duration)
			for {
				select {
				case <-end:
					atomic.AddInt32(&active, -1)
					return
				default:
					math.Sqrt(123456.789)
				}
			}
		}()
	}
	resp := StressResp{
		Workers:    numCPU,
		DurationMs: duration.Milliseconds(),
		Message:    fmt.Sprintf("Spinning %d CPU cores for %v", numCPU, duration),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func moveHandler(w http.ResponseWriter, r *http.Request) {
	player := r.URL.Query().Get("player")
	if player == "" {
		player = "default"
	}
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		dir = "north"
	}
	resp := map[string]string{
		"player":  player,
		"action":  "move",
		"dir":     dir,
		"result":  "ok",
		"pod":     os.Getenv("HOSTNAME"),
		"time":    time.Now().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/move", moveHandler)
	http.HandleFunc("/stress", stressHandler)
	log.Printf("[Game] Starting on :%s (pod=%s, cpus=%d)", port, os.Getenv("HOSTNAME"), runtime.NumCPU())
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
