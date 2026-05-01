package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"ch5/internal/proto"
)

// 固定配置
const (
	mapSize     = 10
	gameAddr0   = "127.0.0.1:8081"
	gameAddr1   = "127.0.0.1:8083"
	storageURL0 = "http://127.0.0.1:8082"
	storageURL1 = "http://127.0.0.1:8084"
)

type shardConfig struct {
	ID   string
	Addr string
}
type storageShard struct {
	ID  string
	URL string
}

// 统一JSON响应
func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

// 封装Storage错误
func sendStorageErr(w http.ResponseWriter, gameCfg shardConfig, store storageShard, userID, mapID string) {
	writeJSON(w, http.StatusBadGateway, proto.ProcessResponse{
		OK:           false,
		Error:        "storage-unreachable",
		UserID:       userID,
		MapID:        mapID,
		GameShard:    gameCfg.ID,
		StorageShard: store.ID,
	})
}

// 自动抢占端口启动Game分片（演示专用，去掉冗余兼容逻辑）
func newGameShard() (net.Listener, shardConfig) {
	// 尝试启动Game-0
	if ln, err := net.Listen("tcp", gameAddr0); err == nil {
		return ln, shardConfig{ID: "Game-0", Addr: gameAddr0}
	}
	// 尝试启动Game-1
	if ln, err := net.Listen("tcp", gameAddr1); err == nil {
		return ln, shardConfig{ID: "Game-1", Addr: gameAddr1}
	}
	log.Fatal("Game端口8081/8083均被占用")
	return nil, shardConfig{}
}

// 分片路由算法（核心，此处total为Storage分片数量，本例为2，即%2哈希）
func parseShardIndex(key string, total int) int {
	num, _ := strconv.Atoi(strings.TrimSpace(key))
	if num < 0 {
		num = -num
	}
	return num % total
}

// 位置边界限制
func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > mapSize {
		return mapSize
	}
	return v
}

// 初始化存储分片
func getStorageShards() []storageShard {
	return []storageShard{
		{"Storage-0", storageURL0},
		{"Storage-1", storageURL1},
	}
}

// 生成存储key
func makeKey(userID, mapID string) string {
	return userID + "|" + mapID
}

// 读取Storage
func storageGet(client *http.Client, baseURL, key string) (proto.Position, error) {
	u, _ := url.Parse(baseURL + "/get")
	q := u.Query()
	q.Set("user_id", key)
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return proto.Position{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return proto.Position{}, fmt.Errorf(string(b))
	}

	var pos proto.Position
	_ = json.NewDecoder(resp.Body).Decode(&pos)
	return pos, nil
}

// 写入Storage
func storageSet(client *http.Client, baseURL, key string, pos proto.Position) error {
	body, _ := json.Marshal(proto.StorageSetRequest{UserID: key, X: pos.X, Y: pos.Y})
	resp, err := client.Post(baseURL+"/set", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf(string(b))
	}
	return nil
}

func main() {
	// 启动Game分片
	ln, gameCfg := newGameShard()
	defer ln.Close()

	storageList := getStorageShards()
	client := &http.Client{Timeout: 2 * time.Second}
	mux := http.NewServeMux()

	// 核心业务接口
	mux.HandleFunc("/process", func(w http.ResponseWriter, r *http.Request) {
		// 1. 基础校验
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, proto.ProcessResponse{OK: false, Error: "method not allowed"})
			return
		}

		// 2. 解析参数
		var req proto.ProcessRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, proto.ProcessResponse{OK: false, Error: "invalid json"})
			return
		}
		req.UserID = strings.TrimSpace(req.UserID)
		req.MapID = strings.TrimSpace(req.MapID)
		req.Action = strings.ToLower(req.Action)

		// 3. 参数合法性校验
		if req.UserID == "" || req.MapID == "" || (req.Action != "get" && req.Action != "move") {
			writeJSON(w, http.StatusBadRequest, proto.ProcessResponse{OK: false, Error: "invalid params"})
			return
		}

		/*
			====================================================================
			【学生重点 1/2】Game：第二层路由（UserID -> Storage）
			- Game 不保存状态
			- 仅按 UserID 精准路由到 Storage-0 / Storage-1
			====================================================================
		*/
		// 4. 路由到Storage分片
		store := storageList[parseShardIndex(req.UserID, len(storageList))]
		log.Printf("[%s] %s | user=%s | map=%s | route->%s", gameCfg.ID, req.Action, req.UserID, req.MapID, store.ID)

		// 5. 读取用户位置
		key := makeKey(req.UserID, req.MapID)
		pos, err := storageGet(client, store.URL, key)
		if err != nil {
			log.Printf("[%s] storage get err: %v", gameCfg.ID, err)
			sendStorageErr(w, gameCfg, store, req.UserID, req.MapID)
			return
		}

		// 6. 移动逻辑
		if req.Action == "move" {
			pos.X = clamp(pos.X + req.DX)
			pos.Y = clamp(pos.Y + req.DY)
			if err := storageSet(client, store.URL, key, pos); err != nil {
				log.Printf("[%s] storage set err: %v", gameCfg.ID, err)
				sendStorageErr(w, gameCfg, store, req.UserID, req.MapID)
				return
			}
		}

		// 7. 返回成功结果
		writeJSON(w, http.StatusOK, proto.ProcessResponse{
			OK:           true,
			UserID:       req.UserID,
			MapID:        req.MapID,
			GameShard:    gameCfg.ID,
			StorageShard: store.ID,
			Position:     pos,
		})
		log.Printf("[%s] success pos=(%d,%d)", gameCfg.ID, pos.X, pos.Y)
	})

	// 启动服务
	log.Printf("[%s] running on %s", gameCfg.ID, gameCfg.Addr)
	if err := http.Serve(ln, mux); err != nil {
		log.Fatal(err)
	}
}
