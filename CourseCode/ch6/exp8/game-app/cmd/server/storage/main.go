package main

import (
	"encoding/json"
	"exp8/game-app/internal/proto"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

type StorageState struct {
	mu        sync.RWMutex
	positions map[string]proto.PlayerState
}

func NewStorageState() *StorageState {
	return &StorageState{positions: make(map[string]proto.PlayerState)}
}

func (s *StorageState) get(playerID string) proto.PlayerState {
	// 未出现过的玩家返回零值坐标 (0,0)，作为初始位置。
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.positions[playerID]
}

func (s *StorageState) set(playerID string, x, y int) proto.PlayerState {
	// Storage 用锁保护 map，保证并发请求写入同一个玩家时不会破坏内存状态。
	s.mu.Lock()
	defer s.mu.Unlock()
	p := proto.PlayerState{X: x, Y: y}
	s.positions[playerID] = p
	return p
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("[storage] failed to write response: %v", err)
	}
}

func main() {
	addr := os.Getenv("STORAGE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8082"
	}

	state := NewStorageState()
	mux := http.NewServeMux()

	// Storage 是玩家位置的唯一真相源，Gateway 和 Game 都不在本地保存坐标。
	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}
		writeJSON(w, http.StatusOK, state.get(id))
	})

	mux.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
			return
		}
		var p proto.PlayerState
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		saved := state.set(id, p.X, p.Y)
		log.Printf("[storage] set id=%s pos=(%d,%d)", id, saved.X, saved.Y)
		writeJSON(w, http.StatusOK, saved)
	})

	log.Printf("[storage] started on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[storage] server exit: %v", err)
	}
}
