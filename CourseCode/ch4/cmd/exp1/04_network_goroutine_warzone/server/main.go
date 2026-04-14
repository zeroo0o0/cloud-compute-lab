package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultAddr = "127.0.0.1:9102"

type session struct {
	player string
	reader *bufio.Reader
	writer *bufio.Writer
	conn   net.Conn
}

type actionEvent struct {
	player  string
	action  string
	dx      int
	dy      int
	delayMS int
}

type actionResult struct {
	event actionEvent
	err   error
}

type playerState struct {
	name string
	x    int
	y    int
	hp   int
}

type gameState struct {
	players map[string]playerState
}

func main() {
	addr := flag.String("addr", defaultAddr, "server listen address")
	flag.Parse()

	if err := run(*addr); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}

func run(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	fmt.Println("goroutine 并发收包：Warzone 极简游戏服务端")
	fmt.Println("场景来自 warzone：连接线程收到玩家 ACTION 后，服务器修改权威 PlayerState，再广播状态。")
	fmt.Println("本演示压缩成一帧：疾风游侠 fast 和断流骑士 slow 都已进入房间并准备。")
	fmt.Println("改进写法：每条玩家连接各自启动 goroutine 读取 ACTION。")
	fmt.Println("监听地址:", addr)

	sessions := map[string]*session{}
	for len(sessions) < 2 {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		s, err := acceptSession(conn)
		if err != nil {
			conn.Close()
			return err
		}
		sessions[s.player] = s
		fmt.Printf("[房间] %s 已加入并准备\n", heroName(s.player))
	}
	defer closeSessions(sessions)

	return runFrameWithGoroutines(newGameState(), sessions)
}

func runFrameWithGoroutines(world gameState, sessions map[string]*session) error {
	for _, s := range sessions {
		if err := writeLine(s.writer, "START_ACTION"); err != nil {
			return err
		}
	}

	start := time.Now()
	results := make(chan actionResult, len(sessions))
	fmt.Println("\nFrame 1：服务器准备处理本帧两个英雄的移动 ACTION。")
	fmt.Println("初始世界状态：", world.render())

	/*
		================ 【学生重点 4/6】Goroutine 解耦：Warzone 游戏嵌入版 ================

		请只看这个 for 循环里的 go func：
		1. 每个玩家连接单独启动一个收包 goroutine。
		2. readAction(s.reader) 的等待发生在各自 goroutine 里。
		3. results channel 把 ACTION 交回主循环，再统一修改权威 PlayerState。

		放到 Warzone 这类游戏里能解决什么问题：
		断流骑士 slow 仍然会晚到，但它只阻塞自己的连接 goroutine；
		疾风游侠 fast 的 ACTION 可以先被服务端应用到世界状态。
		============================================================================
	*/
	for _, s := range sessions {
		go func(s *session) {
			ev, err := readAction(s.reader)
			results <- actionResult{event: ev, err: err}
		}(s)
	}
	fmt.Printf("[%s] 主循环已完成收包任务分发，可以继续推进游戏逻辑。\n", formatDuration(time.Since(start)))

	for i := 0; i < len(sessions); i++ {
		got := <-results
		if got.err != nil {
			return got.err
		}
		world.applyAction(got.event)
		fmt.Printf("[应用 ACTION] %s %-10s 客户端模拟延迟=%dms  服务端生效时刻=%s\n",
			heroName(got.event.player), got.event.action, got.event.delayMS, formatDuration(time.Since(start)))
	}

	broadcastState(world, sessions)
	fmt.Println("\n最终世界状态：", world.render())
	fmt.Println("结论：把网络等待从游戏主循环里拆出去，慢连接不再拖住其他玩家的 ACTION。")
	return nil
}

func newGameState() gameState {
	return gameState{players: map[string]playerState{
		"fast": {name: "疾风游侠 fast", x: 2, y: 2, hp: 100},
		"slow": {name: "断流骑士 slow", x: 2, y: 4, hp: 100},
	}}
}

func (g gameState) applyAction(ev actionEvent) {
	p := g.players[ev.player]
	p.x += ev.dx
	p.y += ev.dy
	g.players[ev.player] = p
}

func (g gameState) render() string {
	fast := g.players["fast"]
	slow := g.players["slow"]
	return fmt.Sprintf("%s=(%d,%d HP=%d), %s=(%d,%d HP=%d)",
		fast.name, fast.x, fast.y, fast.hp,
		slow.name, slow.x, slow.y, slow.hp)
}

func broadcastState(world gameState, sessions map[string]*session) {
	fmt.Println("[广播 STATE_UPDATE] ", world.render())
	for _, s := range sessions {
		_ = writeLine(s.writer, "DONE "+world.render())
	}
}

func acceptSession(conn net.Conn) (*session, error) {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	line, err := readLine(reader)
	if err != nil {
		return nil, err
	}
	parts := strings.Fields(line)
	if len(parts) != 2 || parts[0] != "HELLO" {
		return nil, fmt.Errorf("bad hello: %q", line)
	}
	if parts[1] != "fast" && parts[1] != "slow" {
		return nil, fmt.Errorf("player must be fast or slow")
	}
	return &session{player: parts[1], reader: reader, writer: writer, conn: conn}, nil
}

func readAction(reader *bufio.Reader) (actionEvent, error) {
	line, err := readLine(reader)
	if err != nil {
		return actionEvent{}, err
	}
	parts := strings.Fields(line)
	if len(parts) != 6 || parts[0] != "ACTION" {
		return actionEvent{}, fmt.Errorf("bad action: %q", line)
	}
	dx, err := strconv.Atoi(parts[3])
	if err != nil {
		return actionEvent{}, err
	}
	dy, err := strconv.Atoi(parts[4])
	if err != nil {
		return actionEvent{}, err
	}
	delayMS, err := strconv.Atoi(parts[5])
	if err != nil {
		return actionEvent{}, err
	}
	return actionEvent{player: parts[1], action: parts[2], dx: dx, dy: dy, delayMS: delayMS}, nil
}

func heroName(player string) string {
	if player == "slow" {
		return "断流骑士 slow"
	}
	return "疾风游侠 fast"
}

func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%.0fms", d.Seconds()*1000)
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	return strings.TrimSpace(line), err
}

func writeLine(writer *bufio.Writer, line string) error {
	if _, err := writer.WriteString(line + "\n"); err != nil {
		return err
	}
	return writer.Flush()
}

func closeSessions(sessions map[string]*session) {
	for _, s := range sessions {
		s.conn.Close()
	}
}
