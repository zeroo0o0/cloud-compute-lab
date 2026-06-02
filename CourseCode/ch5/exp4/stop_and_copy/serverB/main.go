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
	mode              = "Stop-and-Copy"
	defaultListenAddr = "127.0.0.1:9101"
	defaultClientAddr = "127.0.0.1:9301"
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

	// Stop-and-Copy 的协议最简单：A 先发 SIZE，再连续发送完整状态字节。
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		fmt.Printf("[Server-B][%s] 读取数据大小失败: %v\n", mode, err)
		return
	}

	size, err := parseSizeLine(line)
	if err != nil {
		fmt.Printf("[Server-B][%s] 解析数据大小失败: %v\n", mode, err)
		return
	}

	fmt.Printf("[Server-B][%s] 准备接收完整状态，size=%s\n", mode, migproto.HumanSize(size))

	// B 读完整个状态后才代表目标服具备恢复玩家的条件。
	start := time.Now()
	n, err := io.CopyN(io.Discard, reader, size)
	if err != nil {
		fmt.Printf("[Server-B][%s] 接收状态数据失败: %v\n", mode, err)
		return
	}
	elapsed := time.Since(start)

	fmt.Printf("[Server-B][%s] 全量状态接收完成，received=%s, elapsed=%s\n",
		mode, migproto.HumanSize(n), elapsed)
	fmt.Printf("[Server-B][%s] 恢复玩家会话，迁移结束\n", mode)
	ready.Store(true)

	// 回 DONE 给 A，让 A 停止停机计时。
	elapsedMs := float64(elapsed.Microseconds()) / 1000.0
	if _, err := fmt.Fprintf(conn, "DONE %.2f\n", elapsedMs); err != nil {
		fmt.Printf("[Server-B][%s] 发送完成确认失败: %v\n", mode, err)
		return
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

func parseSizeLine(line string) (int64, error) {
	parts := strings.Fields(line)
	if len(parts) != 2 || parts[0] != "SIZE" {
		return 0, fmt.Errorf("期望格式 SIZE <bytes>，实际为 %q", strings.TrimSpace(line))
	}
	return strconv.ParseInt(parts[1], 10, 64)
}

func getenv(key string, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
