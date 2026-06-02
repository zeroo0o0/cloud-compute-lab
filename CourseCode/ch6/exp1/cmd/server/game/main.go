package main

import (
	"bytes"
	"ch6/exp1/internal/proto"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const mapSize = 10

func playerID(msg proto.ClientMsg) string {
	if strings.TrimSpace(msg.PlayerID) != "" {
		return strings.TrimSpace(msg.PlayerID)
	}
	return strings.TrimSpace(msg.ClientID)
}

// moveDelta 返回本次位移增量；优先使用 dx/dy，必要时回退到 x/y。
func moveDelta(msg proto.ClientMsg) (int, int) {
	if msg.DX != 0 || msg.DY != 0 {
		return msg.DX, msg.DY
	}
	return msg.X, msg.Y
}

func clampPosition(v int) int {
	if v < 0 {
		return 0
	}
	if v > mapSize {
		return mapSize
	}
	return v
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("[game] failed to write response: %v", err)
	}
}

// 为了课堂演示统一对外错误文案：storage 下游失败统一返回 unreachable。
func storageErrResponse() proto.GameResponse {
	return proto.GameResponse{OK: false, Layer: "storage", Error: "unreachable"}
}

// loadState 通过 HTTP 从 storage 读取玩家状态。
func loadState(client *http.Client, storageURL, pid string) (proto.PlayerState, error) {
	u, err := url.Parse(storageURL + "/get")
	if err != nil {
		return proto.PlayerState{}, err
	}
	q := u.Query()
	q.Set("id", pid)
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return proto.PlayerState{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return proto.PlayerState{}, fmt.Errorf("storage get failed: %s", strings.TrimSpace(string(body)))
	}

	var state proto.PlayerState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return proto.PlayerState{}, err
	}
	return state, nil
}

// saveState 通过 HTTP 把更新后的玩家状态写回 storage。
func saveState(client *http.Client, storageURL, pid string, state proto.PlayerState) error {
	u, err := url.Parse(storageURL + "/set")
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("id", pid)
	u.RawQuery = q.Encode()

	body, err := json.Marshal(state)
	if err != nil {
		return err
	}

	resp, err := client.Post(u.String(), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("storage set failed: %s", strings.TrimSpace(string(b)))
	}
	return nil
}

func main() {
	// game 服务监听地址。
	addr := os.Getenv("GAME_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8081"
	}
	// storage 下游地址。
	storageURL := os.Getenv("STORAGE_URL")
	if storageURL == "" {
		storageURL = "http://127.0.0.1:8082"
	}
	if !strings.HasPrefix(storageURL, "http://") && !strings.HasPrefix(storageURL, "https://") {
		storageURL = "http://" + storageURL
	}

	client := &http.Client{Timeout: 3 * time.Second}
	mux := http.NewServeMux()

	/*
	   ==================【学生重点 2/3】Game：无状态编排者==================

	   只看这条链路：读旧状态 -> 应用逻辑 -> 写回存储。
	   1. 从 Storage 拉取旧状态。
	   2. 在 Game 中计算新坐标。
	   3. 把新状态写回 Storage。

	   核心结论：Game 不保存状态，所有状态都在 Storage。
	   ==================================================================
	*/
	// /move：核心链路（读旧状态 -> 应用逻辑 -> 写回存储），game 自身不保存状态。
	mux.HandleFunc("/move", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, proto.GameResponse{OK: false, Layer: "game", Error: "method not allowed"})
			return
		}

		var msg proto.ClientMsg
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			writeJSON(w, http.StatusBadRequest, proto.GameResponse{OK: false, Layer: "game", Error: "invalid json"})
			return
		}

		pid := playerID(msg)
		if pid == "" {
			writeJSON(w, http.StatusBadRequest, proto.GameResponse{OK: false, Layer: "game", Error: "playerId/clientId is required"})
			return
		}
		log.Printf("[game] move request playerId=%s dx=%d dy=%d", pid, msg.DX, msg.DY)

		// 1. 从存储读取旧状态（网络调用，状态源在 storage）
		state, err := loadState(client, storageURL, pid)
		if err != nil {
			log.Printf("[game] storage load failed: %v", err)
			writeJSON(w, http.StatusBadGateway, storageErrResponse())
			return
		}

		// 2. 执行游戏逻辑（把增量位移应用到状态）
		dx, dy := moveDelta(msg)
		newX := clampPosition(state.X + dx)
		newY := clampPosition(state.Y + dy)
		log.Printf("[game] apply move id=%s (%d,%d)->(%d,%d)", pid, state.X, state.Y, newX, newY)
		state.X = newX
		state.Y = newY

		// 3. 写回存储（网络调用，更新后的状态落盘到 storage）
		if err := saveState(client, storageURL, pid, state); err != nil {
			log.Printf("[game] storage save failed: %v", err)
			writeJSON(w, http.StatusBadGateway, storageErrResponse())
			return
		}

		writeJSON(w, http.StatusOK, proto.GameResponse{
			OK:       true,
			Layer:    "game",
			Result:   "moved",
			PlayerID: pid,
			Position: &state,
		})
	})

	// /player：直接从 storage 拉取当前状态，每轮开始时显示玩家位置。
	mux.HandleFunc("/player", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, proto.GameResponse{OK: false, Layer: "game", Error: "method not allowed"})
			return
		}
		pid := strings.TrimSpace(r.URL.Query().Get("clientId"))
		if pid == "" {
			pid = strings.TrimSpace(r.URL.Query().Get("playerId"))
		}
		if pid == "" {
			writeJSON(w, http.StatusBadRequest, proto.GameResponse{OK: false, Layer: "game", Error: "clientId/playerId is required"})
			return
		}
		log.Printf("[game] get request playerId=%s", pid)

		state, err := loadState(client, storageURL, pid)
		if err != nil {
			log.Printf("[game] storage load failed: %v", err)
			writeJSON(w, http.StatusBadGateway, storageErrResponse())
			return
		}
		writeJSON(w, http.StatusOK, proto.GameResponse{OK: true, Layer: "game", Position: &state})
	})

	log.Printf("[game] started on %s, storage=%s", addr, storageURL)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[game] server exit: %v", err)
	}
}
