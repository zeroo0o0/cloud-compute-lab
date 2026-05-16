package main

import (
	"bufio"
	"bytes"
	"exp5/game-app/internal/proto"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func buildResponse(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		line = "RESULT err layer=gateway reason=empty-response"
	}
	return line + "\n\n"
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

/*
==================【学生重点 1/3】网关：协议转换与路由==================

只看这件事：网关不保存状态，只做“文本协议 -> HTTP 请求”的转换。
1. 解析 TCP 文本命令（GET/MOVE）。
2. 转成 game 的 HTTP 调用（/player 或 /move）。
3. 将 game 响应再转回文本发给客户端。

核心结论：网关是“路由器”，状态不在这里。
====================================================================
*/
// forwardToGame 将网关命令转换为 HTTP 请求并转发给 game 服务。
// 入参 line 协议：GET <clientId> 或 MOVE <clientId> <direction>
func forwardToGame(httpClient *http.Client, gameBase string, line string) string {
	// 1) 命令分帧与解析：按空白切分行协议，得到命令词和参数。
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) == 0 {
		return buildResponse("RESULT err layer=gateway reason=empty-command")
	}

	command := strings.ToUpper(parts[0])

	var req *http.Request //HTTP请求对象
	var err error         //错误变量

	// 2) 协议映射：把 TCP 文本命令映射到具体 HTTP endpoint + method。
	switch command {
	case "GET":
		// GET <clientId> -> GET /player?clientId=...
		if len(parts) != 2 {
			return buildResponse("RESULT err layer=gateway reason=bad-arguments")
		}
		u, parseErr := url.Parse(gameBase + "/player")
		if parseErr != nil {
			return buildResponse("RESULT err layer=gateway reason=invalid-game-url")
		}
		q := u.Query()
		q.Set("clientId", parts[1])
		u.RawQuery = q.Encode()
		req, err = http.NewRequest(http.MethodGet, u.String(), nil)

	case "MOVE":
		// MOVE <clientId> <direction> -> POST /move + JSON body(dx,dy)
		if len(parts) != 3 {
			return buildResponse("RESULT err layer=gateway reason=bad-arguments")
		}

		dx, dy, mapErr := directionToDelta(parts[2])
		if mapErr != nil {
			return buildResponse("RESULT err layer=gateway reason=invalid-direction")
		}

		// 把命令参数封装为业务请求体，供下游 game 服务直接消费。
		payload, marshalErr := json.Marshal(proto.MoveRequest{PlayerID: parts[1], DX: dx, DY: dy})
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

	// 3) 构造失败属于网关本地错误，直接返回明确错误信息。
	if err != nil {
		return buildResponse("RESULT err layer=gateway reason=build-request-failed")
	}

	// 4) 下游调用：统一通过 httpClient 发请求，受超时配置保护。
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[gateway] game call failed: %v", err)
		return buildResponse("RESULT err layer=game reason=unreachable")
	}
	defer resp.Body.Close()

	// 5) 响应归一化：不论下游返回何种格式，先读取成文本再统一处理。
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

func main() {
	// 网关监听地址。
	addr := os.Getenv("GATEWAY_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	// game 服务地址（网关的下游）。
	gameURL := os.Getenv("GAME_URL")
	if gameURL == "" {
		gameURL = "http://127.0.0.1:8081"
	}
	if !strings.HasPrefix(gameURL, "http://") && !strings.HasPrefix(gameURL, "https://") {
		gameURL = "http://" + gameURL
	}

	httpClient := &http.Client{Timeout: 3 * time.Second}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[gateway] listen failed: %v", err)
	}
	defer ln.Close()
	log.Printf("[gateway] started on %s, game=%s", addr, gameURL)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[gateway] accept failed: %v", err)
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			reader := bufio.NewScanner(c)
			for reader.Scan() {
				line := reader.Text()
				// 网关职责是转发，不处理业务状态；收到命令后交给 game 服务。
				log.Printf("[gateway] forward command: %s", line)
				resp := forwardToGame(httpClient, gameURL, line)
				log.Printf("[gateway] response: %s", strings.TrimSpace(resp))
				// 将下游响应原样回写给客户端，形成透明代理链路。
				if _, err := fmt.Fprint(c, resp); err != nil {
					log.Printf("[gateway] write failed: %v", err)
					return
				}
			}
			if err := reader.Err(); err != nil {
				log.Printf("[gateway] read failed: %v", err)
			}
		}(conn)
	}
}
