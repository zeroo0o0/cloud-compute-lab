package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"sync"

	"ch5/internal/proto"
)

// 固定端口配置（演示专用，极简启动）
const (
	StorageAddr0 = "127.0.0.1:8082" // Storage-0 端口
	StorageAddr1 = "127.0.0.1:8084" // Storage-1 端口
)

// shardConfig 分片配置：ID+监听地址
type shardConfig struct {
	ID   string
	Addr string
}

// storageState 线程安全的内存存储
// 保存用户坐标，读写锁防止并发冲突
type storageState struct {
	mu    sync.RWMutex
	store map[string]proto.Position
}

// newStorageState 初始化内存存储
func newStorageState() *storageState {
	return &storageState{store: make(map[string]proto.Position)}
}

// Get 线程安全读取用户坐标
func (s *storageState) Get(key string) proto.Position {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.store[key]
}

// Set 线程安全写入用户坐标
func (s *storageState) Set(key string, p proto.Position) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[key] = p
}

// writeJSON 统一封装JSON响应
func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

// newStorageShard 极简端口抢占：运行两次自动启动两个分片
func newStorageShard() (net.Listener, shardConfig) {
	// 抢占 Storage-0 端口
	if ln, err := net.Listen("tcp", StorageAddr0); err == nil {
		return ln, shardConfig{ID: "Storage-0", Addr: StorageAddr0}
	}
	// 抢占 Storage-1 端口
	if ln, err := net.Listen("tcp", StorageAddr1); err == nil {
		return ln, shardConfig{ID: "Storage-1", Addr: StorageAddr1}
	}
	// 端口均被占用，退出
	log.Fatal("Storage 端口 8082/8084 均被占用")
	return nil, shardConfig{}
}

func main() {
	// 1. 启动存储分片（自动抢占端口）
	ln, cfg := newStorageShard()
	defer ln.Close()

	// 2. 初始化内存数据存储
	state := newStorageState()
	mux := http.NewServeMux()

	/*
		====================================================================
		【学生重点 3】Storage：状态唯一存放处
		- GET 读取用户坐标
		- SET 写入用户坐标
		- 仅做存取，不参与路由或业务逻辑
		====================================================================
	*/
	// ------------------- 接口1：查询用户坐标 GET -------------------
	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		// 获取用户ID
		uid := r.URL.Query().Get("user_id")
		if uid == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}
		// 查询并返回
		pos := state.Get(uid)
		log.Printf("[%s] GET | user=%s | pos=(%d,%d)", cfg.ID, uid, pos.X, pos.Y)
		writeJSON(w, http.StatusOK, pos)
	})

	// ------------------- 接口2：设置用户坐标 POST -------------------
	mux.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		// 解析请求参数
		var req proto.StorageSetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if req.UserID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}
		// 写入数据并返回
		pos := proto.Position{X: req.X, Y: req.Y}
		state.Set(req.UserID, pos)
		log.Printf("[%s] SET | user=%s | pos=(%d,%d)", cfg.ID, req.UserID, pos.X, pos.Y)
		writeJSON(w, http.StatusOK, pos)
	})

	// 3. 启动HTTP服务，监听请求
	log.Printf("[%s] running on %s", cfg.ID, cfg.Addr)
	if err := http.Serve(ln, mux); err != nil {
		log.Fatal(err)
	}
}
