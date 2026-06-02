package main

import (
	"bufio"
	"ch5/exp2/internal/proto"
	render "ch5/exp2/internal/render"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

// ===================== 固定配置 =====================
const (
	mapSize     = 10               // 地图大小
	defaultAddr = "127.0.0.1:8080" // 默认网关地址
	defaultUser = "1"              // 默认用户ID
	defaultMap  = "0"              // 默认地图ID
)

// ===================== 工具函数：基础操作 =====================

// readLine 读取控制台输入（去空格）
func readLine(scanner *bufio.Scanner) (string, bool) {
	if !scanner.Scan() {
		return "", false
	}
	return strings.TrimSpace(scanner.Text()), true
}

// parseDirection 校验方向输入（仅支持w/a/s/d）
func parseDirection(input string) (string, error) {
	dir := strings.ToLower(input)
	switch dir {
	case "w", "a", "s", "d":
		return dir, nil
	default:
		return "", fmt.Errorf("无效方向")
	}
}

// sendCommand 发送TCP命令到网关，接收响应结果
func sendCommand(addr, cmd string) ([]string, error) {
	// 连接网关（超时3秒）
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close() // 函数结束关闭连接
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	// 发送命令
	if _, err := fmt.Fprintln(conn, cmd); err != nil {
		return nil, err
	}

	// 读取网关返回的所有响应
	reader := bufio.NewReader(conn)
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// 读取完毕，跳出循环
				if line = strings.TrimSpace(line); line != "" {
					lines = append(lines, line)
				}
				break
			}
			return lines, err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	return lines, nil
}

// printLines 打印网关返回的响应信息
func printLines(lines []string) {
	for _, line := range lines {
		fmt.Println(line)
	}
}

// ===================== 地图渲染相关：解析坐标 + 绘制地图 =====================

// parsePositionFromResult 从响应中解析坐标字符串
func parsePositionFromResult(lines []string) (string, bool) {
	// 倒序查找成功结果
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if !strings.HasPrefix(line, "RESULT ok") {
			continue
		}
		// 截取坐标部分
		start := strings.Index(line, "position=(") + len("position=(")
		end := strings.Index(line[start:], ")")
		if end < 0 {
			continue
		}
		return line[start : start+end], true
	}
	return "", false
}

// parsePositionXY 解析坐标字符串为x/y数值
func parsePositionXY(position string) (int, int, bool) {
	var x, y int
	_, err := fmt.Sscanf(position, "x=%d,y=%d", &x, &y)
	return x, y, err == nil
}

// renderMapFromLines 解析响应并渲染游戏地图
func renderMapFromLines(lines []string) {
	posStr, ok := parsePositionFromResult(lines)
	if !ok {
		return
	}
	x, y, ok := parsePositionXY(posStr)
	if !ok {
		return
	}
	// 渲染玩家位置
	fmt.Println(render.FormatWorldState([]proto.PlayerState{{X: x, Y: y}}, mapSize, mapSize))
}

// ===================== 主函数：客户端入口 =====================
func main() {
	scanner := bufio.NewScanner(os.Stdin)
	var addr, userID, mapID string

	// 1. 输入网关地址（默认8080）
	fmt.Printf("网关地址 (默认 %s): ", defaultAddr)
	if input, ok := readLine(scanner); ok && input != "" {
		addr = input
	} else {
		addr = defaultAddr
	}

	// 2. 输入用户ID（默认1）
	fmt.Printf("用户ID (默认 %s): ", defaultUser)
	if input, ok := readLine(scanner); ok && input != "" {
		userID = input
	} else {
		userID = defaultUser
	}

	// 3. 输入地图ID（默认0）
	fmt.Printf("地图ID (默认 %s): ", defaultMap)
	if input, ok := readLine(scanner); ok && input != "" {
		mapID = input
	} else {
		mapID = defaultMap
	}

	// 操作说明
	fmt.Println("\n输入说明: w/a/s/d=移动 | map <id>=切地图 | user <id>=切用户 | q=退出")
	showState := true // 首次启动显示状态

	// 4. 循环处理用户命令
	for {
		// 显示当前状态：查询玩家位置并渲染地图
		if showState {
			fmt.Printf("\n当前状态 → 用户:%s  地图:%s\n", userID, mapID)
			// 发送GET命令查询位置
			lines, err := sendCommand(addr, fmt.Sprintf("GET %s %s", userID, mapID))
			if err != nil {
				fmt.Printf("查询失败: %v\n", err)
			} else {
				printLines(lines)
				renderMapFromLines(lines)
			}
			showState = false
		}

		// 读取用户输入命令
		fmt.Print("请输入命令: ")
		input, ok := readLine(scanner)
		if !ok {
			return
		}
		cmd := strings.ToLower(input)

		// 命令处理
		switch {
		// 退出客户端
		case cmd == "q":
			fmt.Println("客户端已退出~")
			return

		// 切换地图：map 1
		case strings.HasPrefix(cmd, "map "):
			if id := strings.Fields(input); len(id) == 2 {
				mapID = id[1]
				showState = true // 切换后刷新状态
			} else {
				fmt.Println("格式错误！正确用法：map 地图ID")
			}

		// 切换用户：user 2
		case strings.HasPrefix(cmd, "user "):
			if id := strings.Fields(input); len(id) == 2 {
				userID = id[1]
				showState = true // 切换后刷新状态
			} else {
				fmt.Println("格式错误！正确用法：user 用户ID")
			}

		// 移动命令：w/a/s/d
		default:
			dir, err := parseDirection(input)
			if err != nil {
				fmt.Println("输入错误！仅支持 w/a/s/d 移动")
				continue
			}
			// 发送MOVE命令到网关
			lines, err := sendCommand(addr, fmt.Sprintf("MOVE %s %s %s", userID, mapID, dir))
			if err != nil {
				fmt.Printf("移动失败: %v\n", err)
				continue
			}
			// 打印结果并渲染地图
			printLines(lines)
			renderMapFromLines(lines)
		}
	}
}
