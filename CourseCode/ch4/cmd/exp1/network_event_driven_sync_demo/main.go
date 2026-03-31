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
	defaultAddr     = "127.0.0.1:9107"
	expectedPlayers = 4
	tickInterval    = 100 * time.Millisecond
	totalTicks      = 8
)

var logMu sync.Mutex

type InputEvent struct {
	PlayerID int
	Seq      int
	Action   string
	DeltaX   int
	LocalAt  time.Duration
	Delay    time.Duration
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

func clientScript(playerID int) []InputEvent {
	switch playerID {
	case 1:
		return []InputEvent{
			{PlayerID: 1, Seq: 1, Action: "MOVE", DeltaX: +1, LocalAt: 0 * time.Millisecond, Delay: 20 * time.Millisecond},
			{PlayerID: 1, Seq: 2, Action: "MOVE", DeltaX: +1, LocalAt: 200 * time.Millisecond, Delay: 20 * time.Millisecond},
		}
	case 2:
		return []InputEvent{
			{PlayerID: 2, Seq: 1, Action: "MOVE", DeltaX: +1, LocalAt: 0 * time.Millisecond, Delay: 30 * time.Millisecond},
			{PlayerID: 2, Seq: 2, Action: "MOVE", DeltaX: +1, LocalAt: 200 * time.Millisecond, Delay: 30 * time.Millisecond},
		}
	case 3:
		return []InputEvent{
			{PlayerID: 3, Seq: 1, Action: "MOVE", DeltaX: -1, LocalAt: 0 * time.Millisecond, Delay: 40 * time.Millisecond},
			{PlayerID: 3, Seq: 2, Action: "MOVE", DeltaX: -1, LocalAt: 200 * time.Millisecond, Delay: 40 * time.Millisecond},
		}
	case 4:
		return []InputEvent{
			{PlayerID: 4, Seq: 1, Action: "MOVE", DeltaX: -1, LocalAt: 0 * time.Millisecond, Delay: 500 * time.Millisecond},
			{PlayerID: 4, Seq: 2, Action: "MOVE", DeltaX: -1, LocalAt: 200 * time.Millisecond, Delay: 500 * time.Millisecond},
		}
	default:
		return nil
	}
}

type incomingEvent struct {
	Event      InputEvent
	ReceivedAt time.Duration
	Err        error
}

func runServer(addr string) error {
	printDivider("实验一 / Network Event-Driven Server")
	logln("监听地址:", addr)
	logln("运行方式: 再打开 1 个终端执行 client 模式。")
	logln("目标: 服务器按 tick 前进，不等最慢客户端，只消费已经到达的事件，并做增量同步。")

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

	for _, session := range sortedSessions(sessions) {
		if err := writeLine(session.Writer, "BEGIN"); err != nil {
			return err
		}
	}

	positions := map[int]int{1: 2, 2: 6, 3: 10, 4: 14}
	eventsCh := make(chan incomingEvent, 32)
	var recvWG sync.WaitGroup
	start := time.Now()

	for _, session := range sortedSessions(sessions) {
		recvWG.Add(1)
		go func(session *ClientSession) {
			defer recvWG.Done()
			if err := receiveLoop(session, start, eventsCh); err != nil {
				eventsCh <- incomingEvent{Err: err}
			}
		}(session)
	}

	go func() {
		recvWG.Wait()
		close(eventsCh)
	}()

	pending := make([]incomingEvent, 0, 8)
	dirty := make(map[int]struct{})
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	nextTick := 1
	for nextTick <= totalTicks {
		select {
		case incoming, ok := <-eventsCh:
			if !ok {
				eventsCh = nil
				continue
			}
			if incoming.Err != nil {
				return incoming.Err
			}
			pending = append(pending, incoming)

		case tickTime := <-ticker.C:
			printDivider(fmt.Sprintf("Tick %d / Network Event-Driven Server", nextTick))
			logf("[主循环] Tick %-2d 到达，当前时间=%s\n", nextTick, formatMS(tickTime.Sub(start)))

			if len(pending) == 0 {
				logln("[处理] 本 tick 没有新事件，服务器继续前进，不等待慢客户端。")
			} else {
				sort.Slice(pending, func(i, j int) bool {
					return pending[i].ReceivedAt < pending[j].ReceivedAt
				})
				logln("[到达] 本 tick 前已到达的事件:")
				for _, item := range pending {
					ev := item.Event
					logf("  - 玩家%-2d seq=%d  %-4s %+d  输入=%-8s 延迟=%-8s 到达=%s\n",
						ev.PlayerID, ev.Seq, ev.Action, ev.DeltaX,
						formatMS(ev.LocalAt), formatMS(ev.Delay), formatMS(item.ReceivedAt))
				}
				for _, item := range pending {
					ev := item.Event
					positions[ev.PlayerID] = clamp(positions[ev.PlayerID]+ev.DeltaX, 0, trackWidth)
					dirty[ev.PlayerID] = struct{}{}
					logf("[处理] 玩家%-2d seq=%d  %-4s %+d  -> 位置=%d\n",
						ev.PlayerID, ev.Seq, ev.Action, ev.DeltaX, positions[ev.PlayerID])
				}
				pending = pending[:0]
			}

			if len(dirty) == 0 {
				logln("[增量同步] 本 tick 无状态变化，不发送增量。")
			} else {
				playerIDs := make([]int, 0, len(dirty))
				for pid := range dirty {
					playerIDs = append(playerIDs, pid)
				}
				sort.Ints(playerIDs)
				logln("[增量同步] 本 tick 只同步发生变化的玩家:")
				for _, pid := range playerIDs {
					logf("  - 玩家%-2d x=%d\n", pid, positions[pid])
				}
				clear(dirty)
			}

			logf("[快照] Tick %-2d 当前世界状态: %s\n", nextTick, renderPositions(positions))
			nextTick++
		}
	}

	for _, session := range sessions {
		if err := writeLine(session.Writer, "DONE"); err != nil {
			return err
		}
	}

	printDivider("Network Event-Driven Server 结束")
	logln("提示: 这里展示的是“谁先到先处理”，慢客户端只会影响自己的更新时刻，不再阻塞下一 tick。")
	return nil
}

func receiveLoop(session *ClientSession, start time.Time, eventsCh chan<- incomingEvent) error {
	for {
		line, err := readLine(session.Reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		ev, err := parseEventLine(line)
		if err != nil {
			return err
		}
		eventsCh <- incomingEvent{
			Event:      ev,
			ReceivedAt: time.Since(start),
		}
	}
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
	return &ClientSession{ID: playerID, Conn: conn, Reader: reader, Writer: writer}, nil
}

func runClient(addr string) error {
	printDivider("实验一 / Network Event-Driven Client")
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

	printDivider("Network Event-Driven Client 结束")
	logln("所有玩家脚本执行完毕。")
	return nil
}

func runOnePlayerClient(addr string, playerID int) error {
	script := clientScript(playerID)
	if len(script) == 0 {
		return fmt.Errorf("玩家%d 没有预设脚本", playerID)
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

	for {
		line, err := readLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		switch line {
		case "BEGIN":
			start := time.Now()
			for _, ev := range script {
				reportAt := ev.LocalAt + ev.Delay
				wait := time.Until(start.Add(reportAt))
				if wait > 0 {
					time.Sleep(wait)
				}
				msg := fmt.Sprintf("EVENT %d %d %s %d %d %d",
					ev.PlayerID, ev.Seq, ev.Action, ev.DeltaX, ev.LocalAt.Milliseconds(), ev.Delay.Milliseconds())
				if err := writeLine(writer, msg); err != nil {
					return err
				}
				logf("[上报] 玩家%-2d seq=%d  输入=%-8s 延迟=%-8s 实际上报=%s  %-4s %+d\n",
					playerID, ev.Seq, formatMS(ev.LocalAt), formatMS(ev.Delay), formatMS(reportAt), ev.Action, ev.DeltaX)
			}
		case "DONE":
			return nil
		default:
			return fmt.Errorf("未知指令: %q", line)
		}
	}
}

func parseEventLine(line string) (InputEvent, error) {
	parts := strings.Fields(line)
	if len(parts) != 7 || parts[0] != "EVENT" {
		return InputEvent{}, fmt.Errorf("非法事件消息: %q", line)
	}

	playerID, err := strconv.Atoi(parts[1])
	if err != nil {
		return InputEvent{}, fmt.Errorf("非法玩家编号: %q", parts[1])
	}
	seq, err := strconv.Atoi(parts[2])
	if err != nil {
		return InputEvent{}, fmt.Errorf("非法序号: %q", parts[2])
	}
	deltaX, err := strconv.Atoi(parts[4])
	if err != nil {
		return InputEvent{}, fmt.Errorf("非法位移: %q", parts[4])
	}
	localAtMS, err := strconv.Atoi(parts[5])
	if err != nil {
		return InputEvent{}, fmt.Errorf("非法输入时刻: %q", parts[5])
	}
	delayMS, err := strconv.Atoi(parts[6])
	if err != nil {
		return InputEvent{}, fmt.Errorf("非法延迟: %q", parts[6])
	}

	return InputEvent{
		PlayerID: playerID,
		Seq:      seq,
		Action:   parts[3],
		DeltaX:   deltaX,
		LocalAt:  time.Duration(localAtMS) * time.Millisecond,
		Delay:    time.Duration(delayMS) * time.Millisecond,
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
	fmt.Println("  go run ./cmd/exp1/network_event_driven_sync_demo server [-addr 127.0.0.1:9107]")
	fmt.Println("  go run ./cmd/exp1/network_event_driven_sync_demo client [-addr 127.0.0.1:9107]")
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
