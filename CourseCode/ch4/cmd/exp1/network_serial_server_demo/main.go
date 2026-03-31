package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	trackWidth      = 20
	defaultAddr     = "127.0.0.1:9101"
	expectedPlayers = 4
	frameBudget     = 100 * time.Millisecond
)

var logMu sync.Mutex

type InputEvent struct {
	PlayerID int
	Action   string
	DeltaX   int
	Latency  time.Duration
}

type ClientSession struct {
	ID     int
	Conn   net.Conn
	Reader *bufio.Reader
	Writer *bufio.Writer
}

func formatMS(d time.Duration) string {
	return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000.0)
}

func logf(format string, args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	fmt.Printf(format, args...)
}

func logln(args ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	fmt.Println(args...)
}

func printDivider(title string) {
	logMu.Lock()
	defer logMu.Unlock()
	fmt.Printf("\n========== %s ==========\n", title)
}

func clamp(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func renderPositions(pos map[int]int) string {
	return fmt.Sprintf("P1(x=%d) P2(x=%d) P3(x=%d) P4(x=%d)", pos[1], pos[2], pos[3], pos[4])
}

func defaultFrames(playerID int) map[int]InputEvent {
	switch playerID {
	case 1:
		return map[int]InputEvent{
			1: {PlayerID: 1, Action: "MOVE", DeltaX: +1, Latency: 11 * time.Millisecond},
			2: {PlayerID: 1, Action: "MOVE", DeltaX: +1, Latency: 11 * time.Millisecond},
		}
	case 2:
		return map[int]InputEvent{
			1: {PlayerID: 2, Action: "MOVE", DeltaX: +1, Latency: 12 * time.Millisecond},
			2: {PlayerID: 2, Action: "MOVE", DeltaX: +1, Latency: 12 * time.Millisecond},
		}
	case 3:
		return map[int]InputEvent{
			1: {PlayerID: 3, Action: "MOVE", DeltaX: -1, Latency: 13 * time.Millisecond},
			2: {PlayerID: 3, Action: "MOVE", DeltaX: -1, Latency: 13 * time.Millisecond},
		}
	case 4:
		return map[int]InputEvent{
			1: {PlayerID: 4, Action: "MOVE", DeltaX: -1, Latency: 500 * time.Millisecond},
			2: {PlayerID: 4, Action: "MOVE", DeltaX: -1, Latency: 500 * time.Millisecond},
		}
	default:
		return map[int]InputEvent{}
	}
}

func runServer(addr string) error {
	printDivider("实验一 / Network Serial Server")
	logln("监听地址:", addr)
	logln("运行方式: 再打开 1 个终端执行 client 模式。")
	logln("目标: 让“慢客户端 -> 服务器串行收包 -> 主循环被拖慢”的链路真实发生。")

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	sessions := make(map[int]*ClientSession, expectedPlayers)
	for len(sessions) < expectedPlayers {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}

		session, err := acceptClient(conn)
		if err != nil {
			conn.Close()
			return err
		}
		if _, exists := sessions[session.ID]; exists {
			session.Conn.Close()
			return fmt.Errorf("玩家%d 重复连接", session.ID)
		}
		sessions[session.ID] = session
		logf("[连接] 玩家%-2d 已连接  (%d/%d)\n", session.ID, len(sessions), expectedPlayers)
	}
	defer closeSessions(sessions)

	positions := map[int]int{1: 2, 2: 6, 3: 10, 4: 14}
	frameOrders := map[int][]int{
		1: {1, 2, 3, 4},
		2: {4, 1, 2, 3},
	}

	for frame := 1; frame <= len(frameOrders); frame++ {
		if err := runServerFrame(frame, frameOrders[frame], positions, sessions); err != nil {
			return err
		}
	}

	for _, session := range sessions {
		if err := writeLine(session.Writer, "DONE"); err != nil {
			return err
		}
	}
	printDivider("Server 结束")
	logln("提示: 现在可以再运行 network_goroutine_server_demo，对比服务器把收包交给独立 goroutine 后，慢客户端是否还会让主循环排队。")
	return nil
}

func acceptClient(conn net.Conn) (*ClientSession, error) {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	line, err := readLine(reader)
	if err != nil {
		return nil, err
	}
	parts := strings.Fields(line)
	if len(parts) != 2 || parts[0] != "HELLO" {
		return nil, fmt.Errorf("非法握手消息: %q", line)
	}

	playerID, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("非法玩家编号: %q", parts[1])
	}
	return &ClientSession{
		ID:     playerID,
		Conn:   conn,
		Reader: reader,
		Writer: writer,
	}, nil
}

func runServerFrame(
	frame int,
	order []int,
	positions map[int]int,
	sessions map[int]*ClientSession,
) error {
	printDivider(fmt.Sprintf("Frame %d / Server", frame))
	logf("[广播] START %d -> 所有客户端开始准备输入\n", frame)
	for _, session := range sortedSessions(sessions) {
		if err := writeLine(session.Writer, fmt.Sprintf("START %d", frame)); err != nil {
			return err
		}
	}

	frameStart := time.Now()
	for _, pid := range order {
		session := sessions[pid]
		logf("[等待] Frame %-2d 按顺序读取 玩家%-2d\n", frame, pid)
		line, err := readLine(session.Reader)
		if err != nil {
			return err
		}

		ev, err := parseEventLine(line)
		if err != nil {
			return err
		}
		if ev.PlayerID != pid {
			return fmt.Errorf("收到的玩家编号与等待顺序不一致: want=%d got=%d", pid, ev.PlayerID)
		}

		waited := time.Since(frameStart)
		logf("[收到] Frame %-2d 玩家%-2d  %-4s %+d  客户端延迟=%-8s 累计等待=%s\n",
			frame, ev.PlayerID, ev.Action, ev.DeltaX, formatMS(ev.Latency), formatMS(waited))
		positions[ev.PlayerID] = clamp(positions[ev.PlayerID]+ev.DeltaX, 0, trackWidth)
	}

	cost := time.Since(frameStart)
	logf("[摘要] Frame %-2d 位置快照: %s\n", frame, renderPositions(positions))
	logf("[摘要] Frame %-2d 主循环耗时: %s (目标<100ms)\n", frame, formatMS(cost))
	if cost > frameBudget {
		logln("[现象] 慢客户端尚未发来输入时，服务器主循环会一直阻塞在对应连接上。")
	}
	return nil
}

func runClient(addr string) error {
	printDivider("实验一 / Network Serial Client")
	logln("连接服务器:", addr)
	logf("当前进程会同时模拟 %d 名玩家，每名玩家保持一条独立连接。\n", expectedPlayers)

	var wg sync.WaitGroup
	errCh := make(chan error, expectedPlayers)

	for playerID := 1; playerID <= expectedPlayers; playerID++ {
		wg.Add(1)
		go func(playerID int) {
			defer wg.Done()
			if err := runOnePlayerClient(addr, playerID); err != nil {
				errCh <- err
			}
		}(playerID)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}
	printDivider("Client 结束")
	logln("所有玩家脚本执行完毕。")
	return nil
}

func runOnePlayerClient(addr string, playerID int) error {
	frames := defaultFrames(playerID)
	if len(frames) == 0 {
		return fmt.Errorf("玩家%d 没有预设脚本，请使用 1~4 号玩家", playerID)
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	if err := writeLine(writer, fmt.Sprintf("HELLO %d", playerID)); err != nil {
		return err
	}
	logf("[客户端%-2d] 已连接服务器并完成握手\n", playerID)

	for {
		line, err := readLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		switch parts[0] {
		case "START":
			if len(parts) != 2 {
				return fmt.Errorf("非法 START 指令: %q", line)
			}
			frame, err := strconv.Atoi(parts[1])
			if err != nil {
				return fmt.Errorf("非法帧编号: %q", parts[1])
			}

			ev, ok := frames[frame]
			if !ok {
				return fmt.Errorf("玩家%d 缺少第%d帧脚本", playerID, frame)
			}

			logf("[客户端%-2d] Frame %-2d 收到 START  模拟延迟=%s\n",
				playerID, frame, formatMS(ev.Latency))
			time.Sleep(ev.Latency)

			msg := fmt.Sprintf("EVENT %d %s %d %d", ev.PlayerID, ev.Action, ev.DeltaX, ev.Latency.Milliseconds())
			if err := writeLine(writer, msg); err != nil {
				return err
			}
			logf("[客户端%-2d] Frame %-2d 已发送 %-4s %+d\n", playerID, frame, ev.Action, ev.DeltaX)
		case "DONE":
			logf("[客户端%-2d] 收到 DONE，退出\n", playerID)
			return nil
		default:
			return fmt.Errorf("未知指令: %q", line)
		}
	}
}

func parseEventLine(line string) (InputEvent, error) {
	parts := strings.Fields(line)
	if len(parts) != 5 || parts[0] != "EVENT" {
		return InputEvent{}, fmt.Errorf("非法事件消息: %q", line)
	}

	playerID, err := strconv.Atoi(parts[1])
	if err != nil {
		return InputEvent{}, fmt.Errorf("非法玩家编号: %q", parts[1])
	}
	deltaX, err := strconv.Atoi(parts[3])
	if err != nil {
		return InputEvent{}, fmt.Errorf("非法位移: %q", parts[3])
	}
	latencyMS, err := strconv.Atoi(parts[4])
	if err != nil {
		return InputEvent{}, fmt.Errorf("非法延迟: %q", parts[4])
	}

	return InputEvent{
		PlayerID: playerID,
		Action:   parts[2],
		DeltaX:   deltaX,
		Latency:  time.Duration(latencyMS) * time.Millisecond,
	}, nil
}

func sortedSessions(sessions map[int]*ClientSession) []*ClientSession {
	ids := make([]int, 0, len(sessions))
	for id := range sessions {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	ordered := make([]*ClientSession, 0, len(ids))
	for _, id := range ids {
		ordered = append(ordered, sessions[id])
	}
	return ordered
}

func closeSessions(sessions map[int]*ClientSession) {
	for _, session := range sessions {
		session.Conn.Close()
	}
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func writeLine(writer *bufio.Writer, line string) error {
	if _, err := writer.WriteString(line + "\n"); err != nil {
		return err
	}
	return writer.Flush()
}

func usage() {
	fmt.Println("用法:")
	fmt.Println("  go run ./cmd/exp1/network_serial_server_demo server [-addr 127.0.0.1:9101]")
	fmt.Println("  go run ./cmd/exp1/network_serial_server_demo client [-addr 127.0.0.1:9101]")
	fmt.Println()
	fmt.Println("建议打开 2 个终端: 1 个 server + 1 个 client。")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "server":
		fs := flag.NewFlagSet("server", flag.ExitOnError)
		addr := fs.String("addr", defaultAddr, "服务器监听地址")
		fs.Parse(os.Args[2:])

		if err := runServer(*addr); err != nil {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	case "client":
		fs := flag.NewFlagSet("client", flag.ExitOnError)
		addr := fs.String("addr", defaultAddr, "服务器地址")
		fs.Parse(os.Args[2:])

		if err := runClient(*addr); err != nil {
			fmt.Fprintf(os.Stderr, "client error: %v\n", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
	}
}
