package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"exp8/game-app/internal/proto"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"exp8/game-app/internal/redismini"
)

const sessionTTLSeconds = 300

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func sessionKey(token string) string {
	// token 是客户端重连时保持不变的身份凭证；session 统一存在 Redis。
	return "session:" + token
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func fieldsToMap(v any) map[string]string {
	out := map[string]string{}
	arr, ok := v.([]any)
	if !ok {
		return out
	}
	for i := 0; i+1 < len(arr); i += 2 {
		out[asString(arr[i])] = asString(arr[i+1])
	}
	return out
}

func buildResponse(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		line = "RESULT err layer=gateway reason=empty-response"
	}
	return line + "\n"
}

func directionToDelta(direction string) (int, int, error) {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "w":
		return 0, -1, nil
	case "s":
		return 0, 1, nil
	case "a":
		return -1, 0, nil
	case "d":
		return 1, 0, nil
	default:
		return 0, 0, fmt.Errorf("invalid direction: %s", direction)
	}
}

func touchSession(rdb *redismini.Client, token string) error {
	// 活跃客户端每次 HELLO/HEARTBEAT 都刷新 TTL，避免 Redis 自动清理在线 session。
	_, err := rdb.Do("EXPIRE", sessionKey(token), strconv.Itoa(sessionTTLSeconds))
	return err
}

func sessionPlayer(rdb *redismini.Client, token string) (string, error) {
	if strings.TrimSpace(token) == "" {
		return "", nil
	}
	raw, err := rdb.Do("HGETALL", sessionKey(token))
	if err != nil {
		return "", err
	}
	fields := fieldsToMap(raw)
	// GET/MOVE 可以省略 playerID，此时从 Redis session 中恢复玩家身份。
	return strings.TrimSpace(fields["player_id"]), nil
}

// forwardToGame converts the TCP text protocol into HTTP calls to game.
// Supported commands:
//
//	GET [playerID]
//	MOVE [playerID] <w|a|s|d>
func forwardToGame(httpClient *http.Client, gameBase string, line string, defaultPlayerID string) string {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) == 0 {
		return buildResponse("RESULT err layer=gateway reason=empty-command")
	}

	command := strings.ToUpper(parts[0])
	var req *http.Request
	var err error

	switch command {
	case "GET":
		// 客户端文本 GET 被网关转换为 Game 的 HTTP /player 查询。
		playerID := defaultPlayerID
		if len(parts) == 2 {
			playerID = parts[1]
		}
		if playerID == "" || len(parts) > 2 {
			return buildResponse("RESULT err layer=gateway reason=bad-arguments")
		}
		u, parseErr := url.Parse(gameBase + "/player")
		if parseErr != nil {
			return buildResponse("RESULT err layer=gateway reason=invalid-game-url")
		}
		q := u.Query()
		q.Set("clientId", playerID)
		u.RawQuery = q.Encode()
		req, err = http.NewRequest(http.MethodGet, u.String(), nil)

	case "MOVE":
		// 客户端文本 MOVE 被网关转换为 Game 的 HTTP /move 请求，网关只做协议转换，不计算坐标。
		playerID := defaultPlayerID
		direction := ""
		switch len(parts) {
		case 2:
			direction = parts[1]
		case 3:
			playerID = parts[1]
			direction = parts[2]
		default:
			return buildResponse("RESULT err layer=gateway reason=bad-arguments")
		}
		if playerID == "" {
			return buildResponse("RESULT err layer=gateway reason=no-session-player")
		}

		dx, dy, mapErr := directionToDelta(direction)
		if mapErr != nil {
			return buildResponse("RESULT err layer=gateway reason=invalid-direction")
		}

		payload, marshalErr := json.Marshal(proto.MoveRequest{PlayerID: playerID, DX: dx, DY: dy})
		if marshalErr != nil {
			return buildResponse("RESULT err layer=gateway reason=marshal-failed")
		}
		req, err = http.NewRequest(http.MethodPost, gameBase+"/move", bytes.NewReader(payload))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}

	default:
		return buildResponse("RESULT err layer=gateway reason=unknown-command")
	}

	if err != nil {
		return buildResponse("RESULT err layer=gateway reason=build-request-failed")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[gateway] game call failed: %v", err)
		return buildResponse("RESULT err layer=game reason=unreachable")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return buildResponse("RESULT err layer=game reason=read-upstream-failed")
	}

	bodyText := strings.TrimSpace(string(body))
	if bodyText == "" {
		bodyText = fmt.Sprintf("{\"status\":%d}", resp.StatusCode)
	}

	var env proto.GameEnvelope
	if err := json.Unmarshal([]byte(bodyText), &env); err != nil {
		return buildResponse("RESULT err layer=game reason=invalid-response")
	}

	if resp.StatusCode >= http.StatusBadRequest || !env.OK {
		layer := strings.TrimSpace(env.Layer)
		if layer == "" {
			layer = "game"
		}
		reason := strings.TrimSpace(env.Error)
		if reason == "" {
			reason = fmt.Sprintf("upstream_status_%d", resp.StatusCode)
		}
		log.Printf("[gateway] upstream returned error: layer=%s reason=%s status=%d", layer, reason, resp.StatusCode)
		return buildResponse(fmt.Sprintf("RESULT err layer=%s reason=%s", layer, reason))
	}

	if env.Position != nil {
		return buildResponse(fmt.Sprintf("RESULT ok position=(x=%d,y=%d)", env.Position.X, env.Position.Y))
	}
	return buildResponse("RESULT ok")
}

func handleConn(conn net.Conn, rdb *redismini.Client, httpClient *http.Client, gameURL string, gatewayID string) {
	defer conn.Close()
	reader := bufio.NewScanner(conn)
	var token string
	var playerID string

	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		switch strings.ToUpper(parts[0]) {
		case "HELLO":
			if len(parts) != 3 {
				fmt.Fprintln(conn, "ERR usage=HELLO token playerID")
				continue
			}
			token = parts[1]
			playerID = parts[2]
			// 任意 Gateway-Pod 都先用 token 去 Redis 查 session。
			// 查得到说明这是断线后的重连，可以由当前 Pod 接手。
			raw, err := rdb.Do("HGETALL", sessionKey(token))
			if err != nil {
				fmt.Fprintf(conn, "ERR redis=%v\n", err)
				return
			}
			fields := fieldsToMap(raw)
			resumed := len(fields) > 0
			// Redis 中有这个 token 的 session，就说明这是断线重连；返回 resumed=true。
			if !resumed {
				// 首次连接才创建 session；不要把玩家状态放到 gateway 内存里。
				if _, err := rdb.Do("HSET", sessionKey(token),
					"player_id", playerID,
					"game_server", gameURL,
					"heartbeat_count", "0",
					"last_seen", strconv.FormatInt(time.Now().Unix(), 10),
				); err != nil {
					fmt.Fprintf(conn, "ERR redis=%v\n", err)
					return
				}
				fields["player_id"] = playerID
				fields["game_server"] = gameURL
				fields["heartbeat_count"] = "0"
			} else {
				playerID = fields["player_id"]
			}
			// 每次 HELLO 都刷新 TTL，避免活跃玩家的 session 被 Redis 过期清理。
			if err := touchSession(rdb, token); err != nil {
				fmt.Fprintf(conn, "ERR redis=%v\n", err)
				return
			}
			log.Printf("[gateway %s] session token=%s player=%s resumed=%t", gatewayID, token, fields["player_id"], resumed)
			fmt.Fprintf(conn, "WELCOME gateway=%s resumed=%t player=%s game=%s heartbeats=%s\n",
				gatewayID, resumed, fields["player_id"], fields["game_server"], fields["heartbeat_count"])

		case "HEARTBEAT":
			if len(parts) != 2 || parts[1] == "" {
				fmt.Fprintln(conn, "ERR usage=HEARTBEAT token")
				continue
			}
			token = parts[1]
			// 心跳计数也写在 Redis；换到新的 Gateway-Pod 后不会从 0 开始。
			count, err := rdb.Do("HINCRBY", sessionKey(token), "heartbeat_count", "1")
			if err != nil {
				fmt.Fprintf(conn, "ERR redis=%v\n", err)
				return
			}
			_, _ = rdb.Do("HSET", sessionKey(token), "last_seen", strconv.FormatInt(time.Now().Unix(), 10))
			_ = touchSession(rdb, token)
			fmt.Fprintf(conn, "PONG gateway=%s heartbeats=%d\n", gatewayID, count)

		case "GET", "MOVE":
			// Game 命令不在 Gateway 本地处理，统一转发给下游 Game 服务。
			defaultPlayerID := playerID
			if defaultPlayerID == "" && token != "" {
				pid, err := sessionPlayer(rdb, token)
				if err != nil {
					fmt.Fprintf(conn, "ERR redis=%v\n", err)
					return
				}
				defaultPlayerID = pid
				playerID = pid
			}
			log.Printf("[gateway %s] forward game command: %s", gatewayID, line)
			fmt.Fprint(conn, forwardToGame(httpClient, gameURL, line, defaultPlayerID))

		default:
			fmt.Fprintf(conn, "ERR unknown=%s\n", parts[0])
		}
	}
	if err := reader.Err(); err != nil {
		log.Printf("[gateway %s] connection error token=%s err=%v", gatewayID, token, err)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "prestop" {
		log.Printf("[gateway] preStop started; waiting 30s before shutdown")
		time.Sleep(30 * time.Second)
		return
	}

	addr := env("GATEWAY_ADDR", "0.0.0.0:8080")
	redisAddr := env("REDIS_ADDR", "127.0.0.1:6379")
	gameURL := env("GAME_URL", "http://127.0.0.1:8081")
	// Kubernetes 中这两个地址由 ConfigMap 注入，本地运行时使用上面的默认值。
	if !strings.HasPrefix(gameURL, "http://") && !strings.HasPrefix(gameURL, "https://") {
		gameURL = "http://" + gameURL
	}
	gatewayID := env("HOSTNAME", "gateway-local")
	rdb := redismini.New(redisAddr)
	httpClient := &http.Client{Timeout: 3 * time.Second}

	if _, err := rdb.Do("PING"); err != nil {
		log.Fatalf("[gateway %s] redis unavailable: %v", gatewayID, err)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[gateway %s] listen failed: %v", gatewayID, err)
	}
	defer ln.Close()
	log.Printf("[gateway %s] started on %s redis=%s game=%s", gatewayID, addr, redisAddr, gameURL)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[gateway %s] accept failed: %v", gatewayID, err)
			continue
		}
		go handleConn(conn, rdb, httpClient, gameURL, gatewayID)
	}
}
