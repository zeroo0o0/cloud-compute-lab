package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

var (
	healthy    int32 = 1 // 1=healthy, 0=deadlocked
	startTime        = time.Now()
	requestCount int64
)

type StatusResp struct {
	Status       string `json:"status"`
	Healthy      bool   `json:"healthy"`
	Uptime       string `json:"uptime"`
	Requests     int64  `json:"requests"`
	PodName      string `json:"pod_name"`
	DeadlockMode bool   `json:"deadlock_mode"`
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	isHealthy := atomic.LoadInt32(&healthy) == 1
	if !isHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "unhealthy",
			"reason": "process deadlocked - /healthz returning 500",
		})
		return
	}
	atomic.AddInt64(&requestCount, 1)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"pod":    os.Getenv("HOSTNAME"),
	})
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	resp := StatusResp{
		Status:       "running",
		Healthy:      atomic.LoadInt32(&healthy) == 1,
		Uptime:       time.Since(startTime).Round(time.Second).String(),
		Requests:     atomic.LoadInt64(&requestCount),
		PodName:      os.Getenv("HOSTNAME"),
		DeadlockMode: atomic.LoadInt32(&healthy) == 0,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func deadlockHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	log.Printf("[WARN] Deadlock triggered! /health will now return 500")
	atomic.StoreInt32(&healthy, 0)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "deadlock_activated",
		"message": "Process is now simulating a deadlock. Liveness probe will detect this and restart the pod.",
	})
}

func recoverHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	log.Printf("[INFO] Manual recovery triggered")
	atomic.StoreInt32(&healthy, 1)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "recovered",
	})
}

func gameHandler(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32(&healthy) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "service deadlocked"})
		return
	}
	atomic.AddInt64(&requestCount, 1)
	player := r.URL.Query().Get("player")
	if player == "" {
		player = "default"
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"player":  player,
		"action":  r.URL.Query().Get("action"),
		"result":  "ok",
		"pod":     os.Getenv("HOSTNAME"),
		"time":    time.Now().Format(time.RFC3339),
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/game", gameHandler)
	http.HandleFunc("/deadlock", deadlockHandler)
	http.HandleFunc("/recover", recoverHandler)

	log.Printf("[Game] Starting on :%s (pod=%s)", port, os.Getenv("HOSTNAME"))
	log.Printf("[Game] Endpoints:")
	log.Printf("  GET  /health    - Liveness probe target")
	log.Printf("  GET  /status    - Service status")
	log.Printf("  GET  /game      - Game API")
	log.Printf("  POST /deadlock  - Simulate process deadlock")
	log.Printf("  POST /recover   - Manual recovery")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
