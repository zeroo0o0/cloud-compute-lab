package main

import (
	"bufio"
	"exp4/internal/migproto"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	mode              = "Wave-Based"
	defaultListenAddr = "127.0.0.1:9103"
	defaultClientAddr = "127.0.0.1:9303"
)

func main() {
	listenAddr := getenv("LISTEN_ADDR", defaultListenAddr)
	clientAddr := getenv("CLIENT_ADDR", defaultClientAddr)
	var ready atomic.Bool
	go serveClientRequests(clientAddr, &ready)

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		fmt.Printf("[Server-B][%s] 启动失败: %v\n", mode, err)
		os.Exit(1)
	}
	defer ln.Close()
	fmt.Printf("[Server-B][%s] 已启动，迁移监听=%s，客户端监听=%s\n", mode, listenAddr, clientAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Printf("[Server-B][%s] Accept 失败: %v\n", mode, err)
			continue
		}
		go handleConn(conn, &ready)
	}
}

func handleConn(conn net.Conn, ready *atomic.Bool) {
	defer conn.Close()
	fmt.Printf("[Server-B][%s] 收到迁移连接: %s\n", mode, conn.RemoteAddr())

	// Wave-Based 会按优先级收到多个阶段：
	// preload_background 是不停机预热，critical_state 是停机期关键状态，
	// background_wave_* 是玩家恢复后的后台补齐。
	reader := bufio.NewReader(conn)
	totalReceived := int64(0)
	for {
		label, size, err := readChunkHeader(reader)
		if err != nil {
			if err == io.EOF {
				fmt.Printf("[Server-B][%s] 本次迁移连接结束，总接收=%s\n", mode, migproto.HumanSize(totalReceived))
				return
			}
			fmt.Printf("[Server-B][%s] 读取阶段头失败: %v\n", mode, err)
			return
		}

		// B 每收完一个阶段就回 DONE；A 用 critical_state 的 DONE 结束停机计时。
		fmt.Printf("[Server-B][%s] 接收 %s，size=%s\n", mode, label, migproto.HumanSize(size))
		start := time.Now()
		n, err := io.CopyN(io.Discard, reader, size)
		if err != nil {
			fmt.Printf("[Server-B][%s] 接收阶段数据失败: %v\n", mode, err)
			return
		}
		elapsed := time.Since(start)
		totalReceived += n
		fmt.Printf("[Server-B][%s] %s 接收完成，received=%s, elapsed=%s\n",
			mode, label, migproto.HumanSize(n), elapsed)
		if label == "critical_state" {
			ready.Store(true)
			fmt.Printf("[Server-B][%s] 关键状态已到达，开始接管客户端请求\n", mode)
		}

		elapsedMs := float64(elapsed.Microseconds()) / 1000.0
		if _, err := fmt.Fprintf(conn, "DONE %s %.2f\n", label, elapsedMs); err != nil {
			fmt.Printf("[Server-B][%s] 发送确认失败: %v\n", mode, err)
			return
		}
	}
}

func serveClientRequests(addr string, ready *atomic.Bool) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("[Server-B][%s] 客户端监听启动失败: %v\n", mode, err)
		return
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Printf("[Server-B][%s] 客户端 Accept 失败: %v\n", mode, err)
			continue
		}
		go handleClient(conn, ready)
	}
}

func handleClient(conn net.Conn, ready *atomic.Bool) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		seq := parseClientSeq(line)
		if !ready.Load() {
			fmt.Fprintf(conn, "NOT_READY B %s\n", seq)
			continue
		}
		fmt.Fprintf(conn, "OK B %s\n", seq)
	}
}

func parseClientSeq(line string) string {
	parts := strings.Fields(line)
	if len(parts) >= 3 {
		return parts[2]
	}
	return "0"
}

func readChunkHeader(reader *bufio.Reader) (string, int64, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", 0, err
	}
	parts := strings.Fields(line)
	if len(parts) != 3 || parts[0] != "CHUNK" {
		return "", 0, fmt.Errorf("期望格式 CHUNK <label> <bytes>，实际为 %q", strings.TrimSpace(line))
	}
	size, err := strconv.ParseInt(parts[2], 10, 64)
	return parts[1], size, err
}

func getenv(key string, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
