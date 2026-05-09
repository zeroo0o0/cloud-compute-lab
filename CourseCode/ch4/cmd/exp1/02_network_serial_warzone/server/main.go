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

const defaultAddr = "127.0.0.1:9101"

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

	fmt.Println("网络串行收包：Warzone 极简游戏服务端")
	fmt.Println("场景来自 warzone：连接线程收到玩家 ACTION 后，服务器修改权威 PlayerState，再广播状态。")
	fmt.Println("本演示压缩成一帧：疾风游侠 fast 和断流骑士 slow 都已进入房间并准备。")
	fmt.Println("错误写法：游戏主循环先读 slow 的 ACTION，再读 fast 的 ACTION。")
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

	return runFrameSerial(newGameState(), sessions)
}

func runFrameSerial(world gameState, sessions map[string]*session) error {
	for _, s := range sessions {
		if err := writeLine(s.writer, "START_ACTION"); err != nil {
			return err
		}
	}

	start := time.Now()
	fmt.Println("\nFrame 1：服务器准备处理本帧两个英雄的移动 ACTION。")
	fmt.Println("初始世界状态：", world.render())

	/*
		================ 【学生重点 2/6】卡顿传染：Warzone 游戏嵌入版 ================

		请只看下面两次 readAction：
		1. readAction(slow)：主循环先等断流骑士 slow 的 ACTION。
		2. readAction(fast)：疾风游侠 fast 可能早已发包，但必须等 slow 返回后才会被读取。

		放到 Warzone 这类游戏里会造成什么问题：
		服务器的权威状态更新被慢连接拖住；fast 的移动明明 20ms 到达，却要排队到 slow 的 500ms 之后才生效。
		这就是“一人卡顿，全员排队”的来源。
		========================================================================
	*/
	slowAction, err := readAction(sessions["slow"].reader)
	if err != nil {
		return err
	}
	world.applyAction(slowAction)
	slowHandledAt := time.Since(start)
	fmt.Printf("[应用 ACTION] %s %-10s 客户端模拟延迟=%dms  服务端生效时刻=%s  额外排队=%s\n",
		heroName(slowAction.player), slowAction.action, slowAction.delayMS,
		formatDuration(slowHandledAt), queueDelay(slowHandledAt, slowAction.delayMS))

	fastAction, err := readAction(sessions["fast"].reader)
	if err != nil {
		return err
	}
	world.applyAction(fastAction)
	fastHandledAt := time.Since(start)
	fmt.Printf("[应用 ACTION] %s %-10s 客户端模拟延迟=%dms  服务端生效时刻=%s  额外排队=%s\n",
		heroName(fastAction.player), fastAction.action, fastAction.delayMS,
		formatDuration(fastHandledAt), queueDelay(fastHandledAt, fastAction.delayMS))

	broadcastState(world, sessions)
	fmt.Println("\n最终世界状态：", world.render())
	fmt.Println("结论：这是反例。游戏主循环不应该按连接顺序串行等待玩家 ACTION。")
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

func queueDelay(handledAt time.Duration, clientDelayMS int) string {
	delay := handledAt - time.Duration(clientDelayMS)*time.Millisecond
	if delay < 0 {
		delay = 0
	}
	return formatDuration(delay)
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
