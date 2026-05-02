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
	"time"
)

const (
	mode              = "Pre-Copy"
	defaultListenAddr = "127.0.0.1:9102"
)

func main() {
	listenAddr := getenv("LISTEN_ADDR", defaultListenAddr)

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		fmt.Printf("[Server-B][%s] 启动失败: %v\n", mode, err)
		os.Exit(1)
	}
	defer ln.Close()
	fmt.Printf("[Server-B][%s] 已启动，监听 %s\n", mode, listenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Printf("[Server-B][%s] Accept 失败: %v\n", mode, err)
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	fmt.Printf("[Server-B][%s] 收到迁移连接: %s\n", mode, conn.RemoteAddr())

	// Pre-Copy 会在同一连接里连续收到多个阶段：
	// round0_full、dirty_round_1/2/3 是不停机预复制，final_dirty 是停机期最后同步。
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

		// 每个 CHUNK 都代表一个迁移阶段，B 收完后立刻回 DONE，让 A 进入下一阶段。
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

		elapsedMs := float64(elapsed.Microseconds()) / 1000.0
		if _, err := fmt.Fprintf(conn, "DONE %s %.2f\n", label, elapsedMs); err != nil {
			fmt.Printf("[Server-B][%s] 发送确认失败: %v\n", mode, err)
			return
		}
	}
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
