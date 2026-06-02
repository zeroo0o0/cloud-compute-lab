package main

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	mode                  = "Pre-Copy"
	initialAddr           = "127.0.0.1:9202"
	requestInterval       = 100 * time.Millisecond
	serverBVerifiedPrefix = "OK B"
)

func main() {
	addr := initialAddr
	conn, reader, err := connect(addr)
	if err != nil {
		fmt.Printf("[Client][%s] 连接 Server-A 失败: %v\n", mode, err)
		return
	}
	defer conn.Close()

	fmt.Printf("[Client][%s] 已启动，模拟玩家每 %s 发送一次操作，当前连接=%s\n", mode, requestInterval.String(), addr)
	for seq := 1; ; seq++ {
		// 每轮循环都发一条动作请求，随后根据服务端回包决定是否切换到 B。
		resp, blocked, err := sendAction(conn, reader, seq)
		if err != nil {
			fmt.Printf("[Client][%s] 请求失败: %v\n", mode, err)
			return
		}

		parts := strings.Fields(resp)
		if len(parts) >= 3 && parts[0] == "REDIRECT" {
			// 收到重定向：说明 A 已经迁移完成，这里主动断开并连到 Server-B。
			nextAddr := parts[1]
			requestWaitMs := float64(blocked.Microseconds()) / 1000.0
			serverDowntimeMs := parseServerDowntime(parts)
			fmt.Printf("\n[Client][%s] action #%d -> 迁移完成，切换到 Server-B=%s，本次请求等待=%.2fms，Server-A停机=%.2fms\n（客户端按 %s 频率发送请求，所以单次等待时间可能小于 Server-A 的完整停机时间。）\n",
				mode, seq, nextAddr, requestWaitMs, serverDowntimeMs, requestInterval.String())
			conn.Close()
			conn, reader, err = connect(nextAddr)
			if err != nil {
				fmt.Printf("[Client][%s] 连接 Server-B 失败: %v\n", mode, err)
				return
			}
			defer conn.Close()
			time.Sleep(requestInterval)
			continue
		}

		// 正常阶段下，Server-A 会直接返回 OK，这里只负责展示延迟。
		printStatus(fmt.Sprintf("[Client][%s] action #%d -> %s, latency=%.2fms",
			mode, seq, strings.TrimSpace(resp), float64(blocked.Microseconds())/1000.0))
		time.Sleep(requestInterval)
	}
}

func printStatus(message string) {
	fmt.Printf("\r\033[2K%s", message)
}

func parseServerDowntime(parts []string) float64 {
	// REDIRECT 回包里会带上 Server-A 的停机时间，便于对比客户端真实等待。
	if len(parts) < 4 {
		return 0
	}
	v, err := strconv.ParseFloat(parts[3], 64)
	if err != nil {
		return 0
	}
	return v
}

func connect(addr string) (net.Conn, *bufio.Reader, error) {
	// 每次切换服务端都重新建立 TCP 连接，并返回一个带缓冲的读取器。
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, nil, err
	}
	return conn, bufio.NewReader(conn), nil
}

func sendAction(conn net.Conn, reader *bufio.Reader, seq int) (string, time.Duration, error) {
	// 发送一条 ACTION 请求，并在服务端没有立即响应时持续等待。
	start := time.Now()
	if _, err := fmt.Fprintf(conn, "ACTION player-heavy-001 %d\n", seq); err != nil {
		return "", 0, err
	}

	reportedWaiting := false
	for {
		// 给这次读取设置短超时，避免迁移期间一直阻塞而没有等待提示。
		_ = conn.SetReadDeadline(time.Now().Add(120 * time.Millisecond))
		line, err := reader.ReadString('\n')
		if err == nil {
			_ = conn.SetReadDeadline(time.Time{})
			return line, time.Since(start), nil
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			if !reportedWaiting {
				printStatus(fmt.Sprintf("[Client][%s] action #%d -> waiting...", mode, seq))
				reportedWaiting = true
			}
			continue
		}
		return "", time.Since(start), err
	}
}
