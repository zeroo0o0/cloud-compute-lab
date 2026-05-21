package main

import (
	"bufio"
	"exp8/game-app/internal/proto"
	"exp8/game-app/internal/render"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

const mapSize = 10

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseDirection(input string) (string, error) {
	dir := strings.ToLower(strings.TrimSpace(input))
	switch dir {
	case "w", "a", "s", "d":
		return dir, nil
	default:
		return "", fmt.Errorf("无效方向: %s（仅支持 w/a/s/d）", input)
	}
}

func readGatewayLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF && strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line), nil
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func parsePosition(line string) (int, int, bool) {
	start := strings.Index(line, "position=(")
	if start < 0 {
		return 0, 0, false
	}
	start += len("position=(")
	end := strings.Index(line[start:], ")")
	if end < 0 {
		return 0, 0, false
	}
	var x, y int
	if _, err := fmt.Sscanf(line[start:start+end], "x=%d,y=%d", &x, &y); err != nil {
		return 0, 0, false
	}
	return x, y, true
}

func isGatewayError(line string) bool {
	return strings.HasPrefix(line, "RESULT err") || strings.HasPrefix(line, "ERR ")
}

func openSession(addr, token, player string) (net.Conn, *bufio.Reader, error) {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, nil, err
	}
	reader := bufio.NewReader(conn)
	// HELLO 是会话恢复入口：断线后仍发送同一个 token，新的 Gateway-Pod 才能从 Redis 找回旧 session。
	if _, err := fmt.Fprintf(conn, "HELLO %s %s\n", token, player); err != nil {
		conn.Close()
		return nil, nil, err
	}
	line, err := readGatewayLine(reader)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	fmt.Println(line)
	return conn, reader, nil
}

func sendCommand(conn net.Conn, reader *bufio.Reader, command string) (string, error) {
	if _, err := fmt.Fprintln(conn, command); err != nil {
		return "", err
	}
	return readGatewayLine(reader)
}

func runHeartbeat(addr, token, player string) error {
	conn, reader, err := openSession(addr, token, player)
	if err != nil {
		return err
	}
	defer conn.Close()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if _, err := fmt.Fprintf(conn, "HEARTBEAT %s\n", token); err != nil {
			return err
		}
		line, err := readGatewayLine(reader)
		if err != nil {
			return err
		}
		log.Printf("[client] %s", line)
	}
	return nil
}

func runHeartbeatLoop(addr, token, player string) {
	for {
		log.Printf("[client] connecting addr=%s token=%s", addr, token)
		if err := runHeartbeat(addr, token, player); err != nil {
			log.Printf("[client] disconnected: %v", err)
			time.Sleep(time.Second)
		}
	}
}

func showPosition(conn net.Conn, reader *bufio.Reader) error {
	line, err := sendCommand(conn, reader, "GET")
	if err != nil {
		return err
	}
	if isGatewayError(line) {
		fmt.Printf("GET失败: %s\n", line)
	} else if x, y, ok := parsePosition(line); ok {
		fmt.Printf("当前位置: x=%d,y=%d\n", x, y)
		fmt.Println(render.FormatWorldState([]proto.PlayerState{{X: x, Y: y}}, mapSize, mapSize))
	} else {
		fmt.Printf("当前位置: %s\n", line)
	}
	return nil
}

func promptInput() {
	fmt.Print("请输入方向 (w/a/s/d)、h 心跳、q 退出: ")
}

func readInputLines() <-chan string {
	lines := make(chan string)
	go func() {
		defer close(lines)
		input := bufio.NewScanner(os.Stdin)
		for input.Scan() {
			lines <- strings.TrimSpace(input.Text())
		}
	}()
	return lines
}

func runGame(addr, token, player string, input <-chan string) error {
	conn, reader, err := openSession(addr, token, player)
	if err != nil {
		return err
	}
	defer conn.Close()

	heartbeat := time.NewTicker(time.Second)
	defer heartbeat.Stop()

	for {
		// 每轮先通过 Gateway 查询当前位置；真实链路是 client -> gateway -> game -> storage。
		if err := showPosition(conn, reader); err != nil {
			return err
		}
		promptInput()

		for {
			select {
			case <-heartbeat.C:
				// 游戏模式下心跳静默发送，避免每秒刷屏；断线会在这里被发现并触发重连。
				if _, err := sendCommand(conn, reader, fmt.Sprintf("HEARTBEAT %s", token)); err != nil {
					fmt.Println()
					return err
				}

			case text, ok := <-input:
				if !ok {
					return nil
				}
				if strings.EqualFold(text, "q") {
					fmt.Println("客户端已退出")
					return nil
				}
				if strings.EqualFold(text, "h") {
					line, err := sendCommand(conn, reader, fmt.Sprintf("HEARTBEAT %s", token))
					if err != nil {
						return err
					}
					fmt.Println(line)
					promptInput()
					continue
				}

				dir, err := parseDirection(text)
				if err != nil {
					fmt.Printf("输入错误: %v\n", err)
					promptInput()
					continue
				}
				line, err := sendCommand(conn, reader, fmt.Sprintf("MOVE %s", dir))
				if err != nil {
					return err
				}
				if isGatewayError(line) {
					fmt.Printf("MOVE失败: %s\n", line)
					promptInput()
					continue
				}
				fmt.Println("移动成功")
				if err := showPosition(conn, reader); err != nil {
					return err
				}
				promptInput()
				break
			}
		}
	}
}

func runGameLoop(addr, token, player string) {
	input := readInputLines()
	for {
		// 当前 TCP 连接断开后，客户端重新连接同一个 NodePort；Service 会把新连接转发到可用 Gateway-Pod。
		if err := runGame(addr, token, player, input); err != nil {
			log.Printf("[client] disconnected: %v", err)
			time.Sleep(time.Second)
			continue
		}
		return
	}
}

func main() {
	addr := env("GATEWAY_ADDR", "127.0.0.1:8080")
	token := env("TOKEN", "student-1-token")
	player := env("PLAYER_ID", "student-1")
	mode := strings.ToLower(env("CLIENT_MODE", "game"))

	if mode == "heartbeat" {
		runHeartbeatLoop(addr, token, player)
		return
	}

	runGameLoop(addr, token, player)
}
