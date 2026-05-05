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
	mode              = "Pre-Copy"
	defaultTargetAddr = "127.0.0.1:9102"
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
	// Pre-Copy 对比重点：
	// 玩家在线时先后台复制 50MB 全量状态，再反复复制变小的脏页。
	// 真正停机时只传最后 1MB 脏页，所以停机时间比 Stop-and-Copy 短；
	// 代价是总传输量会超过 50MB，因为部分状态被重复传输。
	conn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		fmt.Printf("[Server-A][%s] 连接 Server-B 失败: %v\n", mode, err)
		return
	}
	defer conn.Close()
	ackReader := bufio.NewReader(conn)

	totalSent := int64(0)
	fmt.Printf("[Server-A][%s] 准备迁移玩家 player-heavy-001，状态大小=%s\n",
		mode, migproto.HumanSize(migproto.TotalStateBytes))
	fmt.Printf("[Server-A][%s] 玩家继续在线，后台先复制大部分状态\n", mode)

	// 阶段 1：不停机预复制。玩家仍在 A 上服务，这些传输不计入玩家感知停机时间。
	totalSent += sendChunk(conn, ackReader, "round0_full", migproto.TotalStateBytes, 0x21)
	for i, size := range []int64{8 * migproto.MiB, 4 * migproto.MiB, 2 * migproto.MiB} {
		label := fmt.Sprintf("dirty_round_%d", i+1)
		totalSent += sendChunk(conn, ackReader, label, size, byte(0x31+i))
	}

	// 提前准备最后脏页，避免把构造测试数据的开销算进停机时间。
	finalPayload := migproto.BuildPayload(migproto.DirtyPageBytes, 0x42)
	fmt.Printf("[Server-A][%s] 后台预复制完成，开始短暂停机，只同步最后脏页\n", mode)

	// 阶段 2：短暂停机。这里只传最后 1MB 脏页，B 确认后玩家恢复。
	freezeAt := time.Now()
	finalSent := sendPayload(conn, ackReader, "final_dirty", finalPayload)
	downtime := time.Since(freezeAt)
	totalSent += finalSent

	fmt.Printf("[Server-A][%s] 迁移完成，玩家可在 Server-B 恢复\n", mode)
	fmt.Printf("[Server-A][%s] 总传输量=%s，停机期传输量=%s\n",
		mode, migproto.HumanSize(totalSent), migproto.HumanSize(finalSent))
	fmt.Printf("[Server-A][%s] 玩家感知停机时间=%.2fms\n",
		mode, float64(downtime.Microseconds())/1000.0)
}

func sendChunk(conn net.Conn, ackReader *bufio.Reader, label string, size int64, seed byte) int64 {
	payload := migproto.BuildPayload(size, seed)
	return sendPayload(conn, ackReader, label, payload)
}

func sendPayload(conn net.Conn, ackReader *bufio.Reader, label string, payload []byte) int64 {
	size := int64(len(payload))
	fmt.Printf("[Server-A][%s] 发送 %s，size=%s\n", mode, label, migproto.HumanSize(size))

	// 单通道小协议：先发阶段名和长度，再发对应字节；B 收完后回 DONE。
	if _, err := fmt.Fprintf(conn, "CHUNK %s %d\n", label, size); err != nil {
		fmt.Printf("[Server-A][%s] 发送阶段头失败: %v\n", mode, err)
		return 0
	}
	written, err := io.Copy(conn, bytes.NewReader(payload))
	if err != nil {
		fmt.Printf("[Server-A][%s] 发送阶段数据失败: %v\n", mode, err)
		return written
	}
	if err := readDone(ackReader, label); err != nil {
		fmt.Printf("[Server-A][%s] 等待 Server-B 确认失败: %v\n", mode, err)
		return written
	}
	return written
}

func readDone(reader *bufio.Reader, wantLabel string) error {
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	parts := strings.Fields(line)
	if len(parts) != 3 || parts[0] != "DONE" || parts[1] != wantLabel {
		return fmt.Errorf("期望格式 DONE %s <elapsed_ms>，实际为 %q", wantLabel, strings.TrimSpace(line))
	}
	_, err = strconv.ParseFloat(parts[2], 64)
	return err
}

func getenv(key string, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
