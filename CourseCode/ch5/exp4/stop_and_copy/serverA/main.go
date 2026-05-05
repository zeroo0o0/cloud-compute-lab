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
	"time"
)

const (
	mode              = "Stop-and-Copy"
	defaultTargetAddr = "127.0.0.1:9101"
)

func main() {
	targetAddr := getenv("TARGET_ADDR", defaultTargetAddr)

	fmt.Printf("[Server-A][%s] 已启动，目标 Server-B=%s\n", mode, targetAddr)
	fmt.Println("按 Enter 开始迁移，输入 q 退出")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			return
		}

		switch strings.TrimSpace(strings.ToLower(scanner.Text())) {
		case "", "migrate":
			runMigration(targetAddr)
			return
		case "q", "quit", "exit":
			fmt.Println("退出 Server-A")
			return
		default:
			fmt.Println("未知命令，请输入 migrate 或 q")
		}
	}
}

func runMigration(targetAddr string) {
	// Stop-and-Copy 对比重点：
	// 迁移前玩家正常在线；一旦冻结，完整 50MB 状态都要在停机窗口内传给 B。
	// 因此它的总传输量不大，但玩家感知停机时间通常是三种方法里最大的。
	state := migproto.BuildPayload(migproto.TotalStateBytes, 0x11)
	fmt.Printf("[Server-A][%s] 准备迁移玩家 player-heavy-001，状态大小=%s\n",
		mode, migproto.HumanSize(int64(len(state))))

	conn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		fmt.Printf("[Server-A][%s] 连接 Server-B 失败: %v\n", mode, err)
		return
	}
	defer conn.Close()

	fmt.Printf("[Server-A][%s] 正常处理玩家请求\n", mode)

	// 停机窗口从这里开始：玩家输入被冻结，直到 B 收完整状态并回 DONE 才结束。
	freezeAt := time.Now()
	fmt.Printf("[Server-A][%s] 冻结玩家输入，开始全量传输\n", mode)

	if _, err := fmt.Fprintf(conn, "SIZE %d\n", len(state)); err != nil {
		fmt.Printf("[Server-A][%s] 发送数据大小失败: %v\n", mode, err)
		return
	}

	written, err := io.Copy(conn, bytes.NewReader(state))
	if err != nil {
		fmt.Printf("[Server-A][%s] 发送状态数据失败: %v\n", mode, err)
		return
	}

	fmt.Printf("[Server-A][%s] 全量状态已发送，sent=%s，等待 Server-B 恢复完成\n",
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
	fmt.Printf("[Server-A][%s] 迁移完成，玩家可在 Server-B 恢复\n", mode)
	fmt.Printf("[Server-A][%s] 玩家感知停机时间=%.2fms\n",
		mode, float64(downtime.Microseconds())/1000.0)
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
