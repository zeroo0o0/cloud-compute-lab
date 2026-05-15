package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

type FunctionConfig struct {
	Name     string `json:"name"`
	Runtime  string `json:"runtime"`  // "python", "go"
	Handler  string `json:"handler"`  // "handler.py" or "handler"
	Entry    string `json:"entry"`    // "handler" function name
	MemoryMB int    `json:"memory_mb"`
	TimeoutS int    `json:"timeout_s"`
}

type InvokeRequest struct {
	Event   map[string]interface{} `json:"event"`
	Context map[string]interface{} `json:"context"`
}

type InvokeResponse struct {
	StatusCode int         `json:"status_code"`
	Body       interface{} `json:"body"`
	DurationMs int64       `json:"duration_ms"`
	ColdStart  bool        `json:"cold_start"`
}

type FunctionInstance struct {
	config    FunctionConfig
	ready     bool
	lastUsed  time.Time
	useCount  int64
	mu        sync.Mutex
}

var (
	functions = make(map[string]*FunctionInstance)
	totalInvocations int64
	coldStarts       int64
)

func loadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var cfg FunctionConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cfg.MemoryMB == 0 {
		cfg.MemoryMB = 128
	}
	if cfg.TimeoutS == 0 {
		cfg.TimeoutS = 30
	}
	functions[cfg.Name] = &FunctionInstance{
		config: cfg,
		ready:  false,
	}
	log.Printf("[Runtime] Function loaded: %s (%s/%s)", cfg.Name, cfg.Runtime, cfg.Handler)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "loaded",
		"name":   cfg.Name,
	})
}

func invokeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	funcName := r.URL.Query().Get("function")
	if funcName == "" {
		http.Error(w, "?function= required", http.StatusBadRequest)
		return
	}
	fn, ok := functions[funcName]
	if !ok {
		http.Error(w, fmt.Sprintf("function %q not found", funcName), http.StatusNotFound)
		return
	}
	var req InvokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = InvokeRequest{Event: make(map[string]interface{})}
	}
	if req.Event == nil {
		req.Event = make(map[string]interface{})
	}
	if req.Context == nil {
		req.Context = make(map[string]interface{})
	}

	start := time.Now()
	atomic.AddInt64(&totalInvocations, 1)

	cold := !fn.ready
	if cold {
		atomic.AddInt64(&coldStarts, 1)
	}

	result, err := executeFunction(fn, req)
	duration := time.Since(start)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	atomic.AddInt64(&fn.useCount, 1)
	fn.lastUsed = time.Now()
	fn.ready = true

	resp := InvokeResponse{
		StatusCode: 200,
		Body:       result,
		DurationMs: duration.Milliseconds(),
		ColdStart:  cold,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func executeFunction(fn *FunctionInstance, req InvokeRequest) (interface{}, error) {
	switch fn.config.Runtime {
	case "python":
		return executePython(fn, req)
	default:
		return nil, fmt.Errorf("unsupported runtime: %s", fn.config.Runtime)
	}
}

func executePython(fn *FunctionInstance, req InvokeRequest) (interface{}, error) {
	handlerPath := filepath.Join("functions", fn.config.Name, fn.config.Handler)
	if _, err := os.Stat(handlerPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("handler file not found: %s", handlerPath)
	}
	inputJSON, err := json.Marshal(req.Event)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(fn.config.TimeoutS)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", handlerPath, string(inputJSON))
	cmd.Env = append(os.Environ(), fmt.Sprintf("HANDLER=%s", fn.config.Entry))
	stdout, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("function error: %s", string(exitErr.Stderr))
		}
		return nil, err
	}
	var result interface{}
	if err := json.Unmarshal(stdout, &result); err != nil {
		return string(stdout), nil
	}
	return result, nil
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{
		"total_invocations": atomic.LoadInt64(&totalInvocations),
		"cold_starts":       atomic.LoadInt64(&coldStarts),
		"warm_ratio":        0.0,
		"functions":         make([]map[string]interface{}, 0),
	}
	total := atomic.LoadInt64(&totalInvocations)
	if total > 0 {
		stats["warm_ratio"] = float64(total-atomic.LoadInt64(&coldStarts)) / float64(total) * 100
	}
	funcs := stats["functions"].([]map[string]interface{})
	for name, fn := range functions {
		funcs = append(funcs, map[string]interface{}{
			"name":      name,
			"ready":     fn.ready,
			"use_count": atomic.LoadInt64(&fn.useCount),
			"last_used": fn.lastUsed.Format(time.RFC3339),
		})
	}
	stats["functions"] = funcs
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func warmupHandler(w http.ResponseWriter, r *http.Request) {
	funcName := r.URL.Query().Get("function")
	if funcName == "" {
		http.Error(w, "?function= required", http.StatusBadRequest)
		return
	}
	fn, ok := functions[funcName]
	if !ok {
		http.Error(w, fmt.Sprintf("function %q not found", funcName), http.StatusNotFound)
		return
	}
	start := time.Now()
	req := InvokeRequest{Event: map[string]interface{}{"warmup": true}}
	_, err := executeFunction(fn, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fn.ready = true
	fn.lastUsed = time.Now()
	log.Printf("[Runtime] Function %s warmed up in %v", funcName, time.Since(start))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "warmed",
		"function":  funcName,
		"duration":  time.Since(start).String(),
	})
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	http.HandleFunc("/load", loadHandler)
	http.HandleFunc("/invoke", invokeHandler)
	http.HandleFunc("/stats", statsHandler)
	http.HandleFunc("/warmup", warmupHandler)

	log.Printf("[FaaS Runtime] Starting on :%s", port)
	log.Printf("[FaaS Runtime] Endpoints:")
	log.Printf("  POST /load     - Load a function")
	log.Printf("  POST /invoke   - Invoke a function (?function=name)")
	log.Printf("  GET  /stats    - Runtime statistics")
	log.Printf("  GET  /warmup   - Warm up a function (?function=name)")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
