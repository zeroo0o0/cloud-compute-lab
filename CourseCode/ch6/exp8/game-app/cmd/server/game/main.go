package main

import (
	"bytes"
	"encoding/json"
	"exp8/game-app/internal/proto"
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

func storageErrResponse() proto.GameResponse {
	return proto.GameResponse{OK: false, Layer: "storage", Error: "unreachable"}
}

func loadState(client *http.Client, storageURL, pid string) (proto.PlayerState, error) {
	// Game 不读写本地内存状态，每次都从 Storage 拉取玩家当前位置。
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

func saveState(client *http.Client, storageURL, pid string, state proto.PlayerState) error {
	// 移动计算完成后立即写回 Storage，让后续请求和重连后的客户端看到同一份状态。
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
	addr := os.Getenv("GAME_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8081"
	}
	storageURL := os.Getenv("STORAGE_URL")
	if storageURL == "" {
		storageURL = "http://127.0.0.1:8082"
	}
	if !strings.HasPrefix(storageURL, "http://") && !strings.HasPrefix(storageURL, "https://") {
		storageURL = "http://" + storageURL
	}

	client := &http.Client{Timeout: 3 * time.Second}
	mux := http.NewServeMux()

	// Game 是无状态编排者：读 Storage、计算移动、写回 Storage。
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

		// 1. 读取旧坐标。
		state, err := loadState(client, storageURL, pid)
		if err != nil {
			log.Printf("[game] storage load failed: %v", err)
			writeJSON(w, http.StatusBadGateway, storageErrResponse())
			return
		}

		// 2. 应用游戏规则：把方向增量加到旧坐标，并限制在地图边界内。
		dx, dy := moveDelta(msg)
		state.X = clampPosition(state.X + dx)
		state.Y = clampPosition(state.Y + dy)

		// 3. 写回新坐标；Game 自身仍然不保存玩家状态。
		if err := saveState(client, storageURL, pid, state); err != nil {
			log.Printf("[game] storage save failed: %v", err)
			writeJSON(w, http.StatusBadGateway, storageErrResponse())
			return
		}

		log.Printf("[game] moved id=%s pos=(%d,%d)", pid, state.X, state.Y)
		writeJSON(w, http.StatusOK, proto.GameResponse{
			OK:       true,
			Layer:    "game",
			Result:   "moved",
			PlayerID: pid,
			Position: &state,
		})
	})

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

		// 查询也直接读 Storage，避免 Game 副本之间出现各自一份坐标。
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
