// 声明主包，程序入口
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

type BurnResponse struct {
	OK       bool  `json:"ok"`       // 是否成功
	BurnedMS int64 `json:"burnedMs"` // 消耗CPU的毫秒数
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// 编码失败打印日志
		log.Printf("[game] failed to write response: %v", err)
	}
}

// burnCPU 核心函数：【消耗CPU资源】（压测的关键！）
// 参数d：持续消耗CPU的时长
func burnCPU(d time.Duration) {
	deadline := time.Now().Add(d)
	var x uint64 = 1
	// 死循环：直到到达截止时间，一直做数学计算（占满CPU）
	for time.Now().Before(deadline) {
		// 无意义的数学运算，专门用来消耗CPU算力
		x = x*1664525 + 1013904223
	}
	// 忽略变量x，避免编译报错
	_ = x
}

// main 程序主入口（启动HTTP服务）
func main() {
	addr := os.Getenv("GAME_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8081"
	}

	mux := http.NewServeMux()

	// 1. 注册 /burn 接口：【压测接口，被loadgen调用】
	mux.HandleFunc("/burn", func(w http.ResponseWriter, r *http.Request) {
		ms := 250
		if raw := r.URL.Query().Get("ms"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 2000 {
				ms = parsed
			}
		}

		burnCPU(time.Duration(ms) * time.Millisecond)
		// 返回JSON响应
		writeJSON(w, http.StatusOK, BurnResponse{OK: true, BurnedMS: int64(ms)})
	})

	// 2. 注册 /healthz 接口：【健康检查接口，给K8s探针用】
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	log.Printf("[game] started on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[game] server exit: %v", err)
	}
}
