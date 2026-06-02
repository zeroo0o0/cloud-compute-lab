package main

import (
	"bufio"
	"bytes"
	"exp4/internal/migproto"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	mode                     = "Stop-and-Copy"
	defaultTargetAddr        = "127.0.0.1:9101"
	defaultClientAddr        = "127.0.0.1:9201"
	defaultServerBClientAddr = "127.0.0.1:9301"
)

type playerState struct {
	mu         sync.Mutex
	frozen     bool
	migrated   bool
	downtimeMs float64
	resume     chan struct{}
}

func main() {
	targetAddr := getenv("TARGET_ADDR", defaultTargetAddr)
	clientAddr := getenv("CLIENT_ADDR", defaultClientAddr)
	serverBClientAddr := getenv("SERVER_B_CLIENT_ADDR", defaultServerBClientAddr)
	state := &playerState{resume: make(chan struct{})}

	// 客户端连接由后台 goroutine 单独处理，前台只负责控制迁移时机。
	go serveClientRequests(clientAddr, serverBClientAddr, state)

	fmt.Printf("[Server-A][%s] 已启动，目标 Server-B=%s，客户端监听=%s\n", mode, targetAddr, clientAddr)
	fmt.Println("按 Enter 开始迁移，输入 q 退出")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			return
		}

		switch strings.TrimSpace(strings.ToLower(scanner.Text())) {
		case "", "migrate":
			runMigration(targetAddr, state)
			if state.isMigrated() {
				fmt.Println("迁移完成，Server-A 等待 2s 以便 Client 收到重定向，然后自动退出")
				time.Sleep(2 * time.Second)
				return
			}
		case "q", "quit", "exit":
			fmt.Println("退出 Server-A")
			return
		default:
			fmt.Println("未知命令，请输入 migrate 或 q")
		}
	}
}

func runMigration(targetAddr string, player *playerState) {
	// Stop-and-Copy 对比重点：
	// 迁移前玩家正常在线；一旦冻结，完整 1000MB 状态都要在停机窗口内传给 B。
	// 因此它的总传输量不大，但玩家感知停机时间通常是三种方法里最大的。
	payload := migproto.BuildPayload(migproto.TotalStateBytes, 0x11)
	fmt.Printf("[Server-A][%s] 准备迁移玩家 player-heavy-001，状态大小=%s\n",
		mode, migproto.HumanSize(int64(len(payload))))

	conn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		fmt.Printf("[Server-A][%s] 连接 Server-B 失败: %v\n", mode, err)
		return
	}
	defer conn.Close()

	fmt.Printf("[Server-A][%s] 正常处理玩家请求\n", mode)

	// 停机窗口从这里开始：玩家请求要么被阻塞，要么在恢复后被重定向。
	freezeAt := time.Now()
	player.freeze()
	fmt.Printf("[Server-A][%s] ***** 关键传输开始：冻结玩家输入，开始全量传输 *****\n", mode)

	if _, err := fmt.Fprintf(conn, "SIZE %d\n", len(payload)); err != nil {
		fmt.Printf("[Server-A][%s] 发送数据大小失败: %v\n", mode, err)
		return
	}

	written, err := io.Copy(conn, bytes.NewReader(payload))
	if err != nil {
		fmt.Printf("[Server-A][%s] 发送状态数据失败: %v\n", mode, err)
		return
	}

	fmt.Printf("[Server-A][%s]全量状态已发送，sent=%s，等待 Server-B 恢复完成\n",
		mode, migproto.HumanSize(written))

	// DONE 表示 B 已经接收完整状态，可以恢复玩家；此时才结束玩家感知停机时间。
	ackReader := bufio.NewReader(conn)
	ack, err := ackReader.ReadString('\n')
	if err != nil {
		fmt.Printf("[Server-A][%s] 接收确认失败: %v\n", mode, err)
		return
	}
	ackElapsed, err := parseDoneLine(ack)
	if err != nil {
		fmt.Printf("[Server-A][%s] 解析确认失败: %v\n", mode, err)
		return
	}
	_ = ackElapsed

	downtime := time.Since(freezeAt)
	player.markMigrated(downtime)
	fmt.Printf("[Server-A][%s] ***** 关键传输完成：迁移完成，玩家可在 Server-B 恢复 *****\n", mode)
	fmt.Printf("[Server-A][%s] Server-A停机时间=%.2fms\n",
		mode, float64(downtime.Microseconds())/1000.0)
}

func serveClientRequests(addr string, serverBClientAddr string, state *playerState) {
	// 这里只负责接入 TCP 连接；每个连接的读写逻辑交给 handleClient。
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("[Server-A][%s] 客户端监听启动失败: %v\n", mode, err)
		return
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Printf("[Server-A][%s] 客户端 Accept 失败: %v\n", mode, err)
			continue
		}
		go handleClient(conn, serverBClientAddr, state)
	}
}

func handleClient(conn net.Conn, serverBClientAddr string, state *playerState) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		seq := parseClientSeq(line)
		// 已迁移完成：直接告诉客户端切换到 Server-B。
		if state.isMigrated() {
			fmt.Fprintf(conn, "REDIRECT %s %s %.2f\n", serverBClientAddr, seq, state.downtime())
			return
		}
		// 迁移冻结中：等待恢复信号后，再把客户端引导到 B。
		if state.isFrozen() {
			state.waitResume()
			fmt.Fprintf(conn, "REDIRECT %s %s %.2f\n", serverBClientAddr, seq, state.downtime())
			return
		}
		// 正常在线阶段：Server-A 直接处理这条操作。
		fmt.Fprintf(conn, "OK A %s\n", seq)
	}
}

func parseClientSeq(line string) string {
	// 客户端请求格式是 ACTION <playerId> <seq>，这里只取序号用于回包。
	parts := strings.Fields(line)
	if len(parts) >= 3 {
		return parts[2]
	}
	return "0"
}

func (s *playerState) freeze() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frozen = true
}

func (s *playerState) markMigrated(downtime time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.migrated {
		s.migrated = true
		s.downtimeMs = float64(downtime.Microseconds()) / 1000.0
		// 关闭通道会唤醒所有在冻结期间等待的客户端请求。
		close(s.resume)
	}
}

func (s *playerState) isFrozen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.frozen
}

func (s *playerState) isMigrated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.migrated
}

func (s *playerState) waitResume() {
	<-s.resume
}

func (s *playerState) downtime() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.downtimeMs
}

func parseDoneLine(line string) (float64, error) {
	parts := strings.Fields(line)
	if len(parts) != 2 || parts[0] != "DONE" {
		return 0, fmt.Errorf("期望格式 DONE <elapsed_ms>，实际为 %q", strings.TrimSpace(line))
	}
	return strconv.ParseFloat(parts[1], 64)
}

func getenv(key string, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
