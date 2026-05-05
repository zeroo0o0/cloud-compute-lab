package main

import (
	"bufio"
	"bytes"
	"exp4/internal/migproto"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	mode              = "Wave-Based"
	defaultTargetAddr = "127.0.0.1:9103"
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
	// Wave-Based 对比重点：
	// 先后台传非关键状态；停机时只传玩家恢复所必需的关键状态；
	// 玩家恢复后，再后台补齐剩余状态。因此停机期传输量最小，玩家感知停机时间也最短。
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
	fmt.Printf("[Server-A][%s] 玩家继续在线，先后台预热非关键状态\n", mode)

	// 阶段 1：不停机预热。先把一部分非关键状态放到 B，玩家仍在 A 上正常服务。
	preloadSize := int64(20 * migproto.MiB)
	totalSent += sendChunk(conn, ackReader, "preload_background", preloadSize, 0x51)

	// 提前准备关键状态，停机后只统计真正传关键状态和等待 B 确认的时间。
	criticalPayload := migproto.BuildPayload(migproto.CriticalStateBytes, 0x66)
	fmt.Printf("[Server-A][%s] 冻结玩家输入，只传关键状态\n", mode)

	// 阶段 2：极短停机。只传 256KB 关键状态，B 收到后玩家即可恢复。
	freezeAt := time.Now()
	criticalSent := sendPayload(conn, ackReader, "critical_state", criticalPayload)
	downtime := time.Since(freezeAt)
	totalSent += criticalSent

	fmt.Printf("[Server-A][%s] 关键状态到达，玩家可在 Server-B 恢复\n", mode)
	fmt.Printf("[Server-A][%s] 玩家感知停机时间=%.2fms\n",
		mode, float64(downtime.Microseconds())/1000.0)

	remaining := migproto.TotalStateBytes - preloadSize - migproto.CriticalStateBytes
	wave2 := int64(math.Round(float64(remaining) * 0.4))
	wave3 := int64(math.Round(float64(remaining) * 0.35))
	waves := []int64{wave2, wave3, remaining - wave2 - wave3}

	// 阶段 3：玩家已经恢复，剩余状态继续在后台补齐，不再计入玩家感知停机时间。
	fmt.Printf("[Server-A][%s] 玩家已恢复，后台继续补齐剩余状态\n", mode)
	for i, size := range waves {
		label := fmt.Sprintf("background_wave_%d", i+2)
		totalSent += sendChunk(conn, ackReader, label, size, byte(0x71+i))
	}

	fmt.Printf("[Server-A][%s] 迁移完成，总传输量=%s，停机期传输量=%s\n",
		mode, migproto.HumanSize(totalSent), migproto.HumanSize(criticalSent))
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
