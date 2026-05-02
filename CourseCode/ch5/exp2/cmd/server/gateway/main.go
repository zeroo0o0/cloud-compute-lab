package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ch5/exp2/internal/proto"
)

// 固定端口配置（演示专用，极简启动）
const (
	GatewayAddr = "127.0.0.1:8080"        // 网关监听端口
	GameURL0    = "http://127.0.0.1:8081" // Game-0 地址
	GameURL1    = "http://127.0.0.1:8083" // Game-1 地址
)

// gameShard Game分片配置
type gameShard struct {
	ID  string
	URL string
}

// ------------------- 工具函数 -------------------
// parseShardIndex 核心分片路由算法：根据MapID计算目标Game分片索引
// 规则：数字ID取模路由，保证同一MapID始终路由到同一分片
// total为Game分片数量，本项目中为2
func parseShardIndex(key string, total int) int {
	num, _ := strconv.Atoi(strings.TrimSpace(key))
	if num < 0 {
		num = -num
	}
	return num % total
}

// directionToDelta 方向转坐标增量
func directionToDelta(dir string) (int, int, error) {
	switch strings.ToLower(dir) {
	case "w":
		return 0, -1, nil
	case "s":
		return 0, 1, nil
	case "a":
		return -1, 0, nil
	case "d":
		return 1, 0, nil
	default:
		return 0, 0, fmt.Errorf("invalid direction")
	}
}

// buildTextResponse 统一格式化客户端响应
func buildTextResponse(lines []string) string {
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			clean = append(clean, line)
		}
	}
	if len(clean) == 0 {
		clean = append(clean, "RESULT err reason=empty-response")
	}
	return strings.Join(clean, "\n") + "\n\n"
}

// response 封装重复调用
func response(routes []string, msg string) string {
	return buildTextResponse(append(routes, msg))
}

// buildGameShards 初始化Game集群
func buildGameShards() []gameShard {
	return []gameShard{
		{ID: "Game-0", URL: GameURL0},
		{ID: "Game-1", URL: GameURL1},
	}
}

// ------------------- 核心：处理客户端命令 -------------------
func handleCommand(client *http.Client, shards []gameShard, line string) string {
	// 解析命令
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) == 0 {
		return response(nil, "RESULT err reason=empty-command")
	}

	cmd := strings.ToUpper(parts[0])
	// 命令合法性校验
	if cmd != "GET" && cmd != "MOVE" {
		return response(nil, "RESULT err reason=unknown-command")
	}
	if (cmd == "GET" && len(parts) != 3) || (cmd == "MOVE" && len(parts) != 4) {
		usage := "usage-GET-user-map"
		if cmd == "MOVE" {
			usage = "usage-MOVE-user-map-dir"
		}
		return response(nil, "RESULT err reason="+usage)
	}

	// 提取参数
	userID, mapID := parts[1], parts[2]
	if userID == "" || mapID == "" {
		return response(nil, "RESULT err reason=empty-user-or-map")
	}

	/*
		====================================================================
		【学生重点 1】网关：只做协议转换 + 第一层路由（MapID -> Game）
		- 解析 TCP 文本命令（GET/MOVE）
		- 按通过哈希算法以 MapID 将请求分发到 Game-0 / Game-1
		- 接收game返回信息，将分片路由结果原路写回给客户端
		====================================================================
	*/
	// 路由：MapID → Game分片
	game := shards[parseShardIndex(mapID, len(shards))]
	log.Printf("[gateway] %s | user=%s | map=%s | route->%s", strings.ToLower(cmd), userID, mapID, game.ID)
	routes := []string{fmt.Sprintf("[gateway] map_id=%s -> %s", mapID, game.ID)}

	// 构造转发请求
	req := proto.ProcessRequest{Action: strings.ToLower(cmd), UserID: userID, MapID: mapID}
	if cmd == "MOVE" {
		dx, dy, err := directionToDelta(parts[3])
		if err != nil {
			return response(routes, "RESULT err reason=invalid-direction")
		}
		req.DX, req.DY = dx, dy
	}

	// 转发请求到Game服务
	body, _ := json.Marshal(req)
	resp, err := client.Post(game.URL+"/process", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[gateway] call game failed: %v", err)
		return response(routes, "RESULT err reason=game-unreachable")
	}
	defer resp.Body.Close()

	// 解析Game响应
	respBody, _ := io.ReadAll(resp.Body)
	var out proto.ProcessResponse
	_ = json.Unmarshal(respBody, &out)

	// 追加Storage分片信息（从Game响应中获取，网关不计算）
	if out.StorageShard != "" {
		routes = append(routes, fmt.Sprintf("[game] user_id=%s -> %s", userID, out.StorageShard))
	}

	// 错误处理
	if resp.StatusCode >= http.StatusBadRequest || !out.OK {
		reason := out.Error
		if reason == "" {
			reason = "game-error"
		}
		log.Printf("[gateway] error: %s", reason)
		return response(routes, fmt.Sprintf("RESULT err reason=%s", reason))
	}

	// 成功返回
	pos := out.Position
	log.Printf("[gateway] success pos=(%d,%d)", pos.X, pos.Y)
	return response(routes, fmt.Sprintf("RESULT ok position=(x=%d,y=%d)", pos.X, pos.Y))
}

// ------------------- 主函数：启动网关 -------------------
func main() {
	shards := buildGameShards()
	client := &http.Client{Timeout: 2 * time.Second}

	// 监听TCP端口
	ln, err := net.Listen("tcp", GatewayAddr)
	if err != nil {
		log.Fatalf("[gateway] listen failed: %v", err)
	}
	defer ln.Close()

	log.Printf("[gateway] running on %s | Game-0=%s | Game-1=%s", GatewayAddr, GameURL0, GameURL1)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[gateway] accept failed: %v", err)
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			scanner := bufio.NewScanner(c)
			for scanner.Scan() {
				// 处理客户端命令
				_, _ = fmt.Fprint(c, handleCommand(client, shards, scanner.Text()))
			}
		}(conn)
	}
}
