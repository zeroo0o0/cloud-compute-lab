package main

import (
	"bufio"
	"ch6/exp1/internal/proto"
	render "ch6/exp1/internal/render"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

const mapSize = 10

func readLine(scanner *bufio.Scanner) (string, bool) {
	if !scanner.Scan() {
		return "", false
	}
	return strings.TrimSpace(scanner.Text()), true
}

func parseDirection(input string) (string, error) {
	dir := strings.ToLower(strings.TrimSpace(input))
	if len(dir) != 1 {
		return "", fmt.Errorf("请输入单个方向字符: w/a/s/d")
	}

	switch dir {
	case "w", "a", "s", "d":
		return dir, nil
	default:
		return "", fmt.Errorf("无效方向: %s（仅支持 w/a/s/d）", input)
	}
}

func sendCommand(addr string, cmd string) ([]string, error) {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	if _, err := fmt.Fprintln(conn, cmd); err != nil {
		return nil, err
	}

	reader := bufio.NewReader(conn)
	lines := make([]string, 0, 12)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" {
					lines = append(lines, trimmed)
				}
				break
			}
			return lines, err
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		lines = append(lines, trimmed)
	}
	return lines, nil
}

func hasResultErr(lines []string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, "RESULT err") {
			return true
		}
	}
	return false
}

func firstResultErr(lines []string) string {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "RESULT err") {
			return trimmed
		}
	}
	return ""
}

func parsePositionFromResult(lines []string) (string, bool) {
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if !strings.HasPrefix(line, "RESULT ok") {
			continue
		}
		start := strings.Index(line, "position=(")
		if start < 0 {
			continue
		}
		start += len("position=(")
		end := strings.Index(line[start:], ")")
		if end < 0 {
			continue
		}
		return line[start : start+end], true
	}
	return "", false
}

func parsePositionXY(position string) (int, int, bool) {
	var x, y int
	if _, err := fmt.Sscanf(position, "x=%d,y=%d", &x, &y); err != nil {
		return 0, 0, false
	}
	return x, y, true
}

func resolveClientID() string {
	if id := strings.TrimSpace(os.Getenv("CLIENT_ID")); id != "" {
		return id
	}
	return fmt.Sprintf("client-%d", os.Getpid())
}

func main() {
	defaultURL := "127.0.0.1:8080"
	if envURL := strings.TrimSpace(os.Getenv("CLIENT_SERVER_URL")); envURL != "" {
		defaultURL = envURL
	}

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("网关地址 (默认 %s): ", defaultURL)
	inputURL, ok := readLine(scanner)
	if !ok {
		return
	}
	baseURL := defaultURL
	if inputURL != "" {
		baseURL = inputURL
	}
	clientID := resolveClientID()
	fmt.Printf("客户端ID: %s\n", clientID)

	for {
		// 1. 先向网关发送 GET <clientId>，由网关转发为 /player?clientId=... 查询当前位置
		respLines, err := sendCommand(baseURL, fmt.Sprintf("GET %s", clientID))
		if err != nil {
			if msg := firstResultErr(respLines); msg != "" {
				fmt.Printf("GET失败: %s\n", msg)
			} else {
				fmt.Printf("GET失败: %v\n", err)
			}
			fmt.Print("请输入方向 (w/a/s/d) 或 q 退出: ")
			input, ok := readLine(scanner)
			if !ok {
				return
			}
			if strings.ToLower(input) == "q" {
				fmt.Println("客户端已退出")
				return
			}
			continue
		}

		if hasResultErr(respLines) {
			if msg := firstResultErr(respLines); msg != "" {
				fmt.Printf("GET失败: %s\n", msg)
			}
			fmt.Print("请输入方向 (w/a/s/d) 或 q 退出: ")
			input, ok := readLine(scanner)
			if !ok {
				return
			}
			if strings.ToLower(input) == "q" {
				fmt.Println("客户端已退出")
				return
			}
			dir, parseErr := parseDirection(input)
			if parseErr != nil {
				fmt.Printf("输入错误: %v\n", parseErr)
				continue
			}

			moveLines, moveErr := sendCommand(baseURL, fmt.Sprintf("MOVE %s %s", clientID, dir))
			if moveErr != nil {
				if msg := firstResultErr(moveLines); msg != "" {
					fmt.Printf("MOVE失败: %s\n", msg)
				}
				fmt.Printf("移动失败: %v\n", moveErr)
				continue
			}
			if hasResultErr(moveLines) {
				if msg := firstResultErr(moveLines); msg != "" {
					fmt.Printf("MOVE失败: %s\n", msg)
				}
			} else {
				fmt.Printf("移动成功\n")
			}
			continue
		}
		if position, ok := parsePositionFromResult(respLines); ok {
			fmt.Printf("当前位置: %s\n", position)
			if x, y, ok := parsePositionXY(position); ok {
				players := []proto.PlayerState{{X: x, Y: y}}
				fmt.Println(render.FormatWorldState(players, mapSize, mapSize))
			}
		} else {
			fmt.Println("当前位置: 未知")
		}

		// 2. 提示用户输入方向或退出
		fmt.Print("请输入方向 (w/a/s/d) 或 q 退出: ")
		input, ok := readLine(scanner)
		if !ok {
			return
		}

		// 3. 处理退出
		if strings.ToLower(input) == "q" {
			fmt.Println("客户端已退出")
			return
		}

		// 4. 验证并执行MOVE
		dir, err := parseDirection(input)
		if err != nil {
			fmt.Printf("输入错误: %v\n", err)
			continue
		}

		moveLines, err := sendCommand(baseURL, fmt.Sprintf("MOVE %s %s", clientID, dir))
		if err != nil {
			if msg := firstResultErr(moveLines); msg != "" {
				fmt.Printf("MOVE失败: %s\n", msg)
			}
			fmt.Printf("移动失败: %v\n", err)
			continue
		}
		if hasResultErr(moveLines) {
			if msg := firstResultErr(moveLines); msg != "" {
				fmt.Printf("MOVE失败: %s\n", msg)
			}
			continue
		}
		fmt.Printf("移动成功\n")
		// 下一个循环将自动获取并显示新位置
	}
}
