package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
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

func touchSession(rdb *redismini.Client, token string) error {
	_, err := rdb.Do("EXPIRE", sessionKey(token), strconv.Itoa(sessionTTLSeconds))
	return err
}

func handleConn(conn net.Conn, rdb *redismini.Client, gatewayID string) {
	defer conn.Close()
	reader := bufio.NewScanner(conn)
	var token string

	for reader.Scan() {
		parts := strings.Fields(strings.TrimSpace(reader.Text()))
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
			playerID := parts[2]
			// 任意 Gateway-Pod 都先用 token 去 Redis 查 session。
			// 查得到说明这是断线后的重连，可以由当前 Pod 接手。
			raw, err := rdb.Do("HGETALL", sessionKey(token))
			if err != nil {
				fmt.Fprintf(conn, "ERR redis=%v\n", err)
				return
			}
			fields := fieldsToMap(raw)
			resumed := len(fields) > 0
			if !resumed {
				// 首次连接才创建 session；不要把玩家状态放到 gateway 内存里。
				if _, err := rdb.Do("HSET", sessionKey(token),
					"player_id", playerID,
					"game_server", "game-1",
					"heartbeat_count", "0",
					"last_seen", strconv.FormatInt(time.Now().Unix(), 10),
				); err != nil {
					fmt.Fprintf(conn, "ERR redis=%v\n", err)
					return
				}
				fields["player_id"] = playerID
				fields["game_server"] = "game-1"
				fields["heartbeat_count"] = "0"
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

		default:
			fmt.Fprintf(conn, "ERR unknown=%s\n", parts[0])
		}
	}
	if err := reader.Err(); err != nil {
		log.Printf("[gateway %s] connection error token=%s err=%v", gatewayID, token, err)
	}
}

func main() {
	addr := env("GATEWAY_ADDR", "0.0.0.0:8080")
	redisAddr := env("REDIS_ADDR", "127.0.0.1:6379")
	gatewayID := env("HOSTNAME", "gateway-local")
	rdb := redismini.New(redisAddr)

	if _, err := rdb.Do("PING"); err != nil {
		log.Fatalf("[gateway %s] redis unavailable: %v", gatewayID, err)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[gateway %s] listen failed: %v", gatewayID, err)
	}
	defer ln.Close()
	log.Printf("[gateway %s] started on %s redis=%s", gatewayID, addr, redisAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[gateway %s] accept failed: %v", gatewayID, err)
			continue
		}
		go handleConn(conn, rdb, gatewayID)
	}
}
