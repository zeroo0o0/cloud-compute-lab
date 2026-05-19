package main

import (
	"ch6/exp1/internal/proto"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type StorageState struct {
	mu        sync.RWMutex
	positions map[string]proto.Position
}

func NewStorageState() *StorageState {
	return &StorageState{positions: make(map[string]proto.Position)}
}

func (s *StorageState) get(clientID string) proto.Position {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.positions[clientID]
}

func (s *StorageState) set(clientID string, x, y int) proto.Position {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := proto.Position{X: x, Y: y}
	s.positions[clientID] = p
	return p
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("[storage] failed to write response: %v", err)
	}
}

func resolveLogPath() string {
	logPath := strings.TrimSpace(os.Getenv("STORAGE_LOG_PATH"))
	if logPath != "" {
		return logPath
	}
	return "/app/data/players.log"
}

func ensureLogDir(logPath string) error {
	dir := filepath.Dir(logPath)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func appendPlayerLog(logPath, playerID string, pos proto.Position) {
	if strings.TrimSpace(logPath) == "" {
		return
	}
	line := time.Now().Format(time.RFC3339) + " id=" + playerID +
		" x=" + strconv.Itoa(pos.X) + " y=" + strconv.Itoa(pos.Y) + "\n"
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("[storage] open log file failed: %v", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		log.Printf("[storage] append log failed: %v", err)
	}
}

func main() {
	addr := os.Getenv("STORAGE_ADDR")
	if addr == "" {
		// 同机直连示例（仅本机运行没问题，Docker 容器内会导致外部无法访问）：
		// addr = "127.0.0.1:8082"
		addr = "0.0.0.0:8082"
	}
	logPath := resolveLogPath()
	if err := ensureLogDir(logPath); err != nil {
		log.Fatalf("[storage] prepare log dir failed: %v", err)
	}

	state := NewStorageState()
	mux := http.NewServeMux()
	/*
	   ==================【学生重点 3/3】Storage：状态权威==================

	   只看两件事：
	   1. /get 读取坐标
	   2. /set 写入坐标

	   核心结论：唯一真相源在 Storage，其他服务只读写它。
	   ================================================================
	*/
	// 关键点：Storage 是唯一的权威状态源。
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
		pos := state.get(id)
		writeJSON(w, http.StatusOK, pos)
	})

	// 写入由 Game 发起，Storage 只做持久化与并发控制。
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
		var p proto.Position
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		saved := state.set(id, p.X, p.Y)
		appendPlayerLog(logPath, id, saved)
		log.Printf("[storage] set id=%s pos=(%d,%d)", id, saved.X, saved.Y)
		writeJSON(w, http.StatusOK, saved)
	})

	log.Printf("[storage] started on %s, log=%s", addr, logPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[storage] server exit: %v", err)
	}
}
