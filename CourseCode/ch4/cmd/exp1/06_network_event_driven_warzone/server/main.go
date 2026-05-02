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

const (
	defaultAddr  = "127.0.0.1:9103"
	tickInterval = 100 * time.Millisecond
	totalTicks   = 6
)

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

type incomingAction struct {
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

	fmt.Println("事件驱动 + 增量同步：Warzone 极简游戏服务端")
	fmt.Println("场景来自 warzone：连接线程把玩家 ACTION 交给游戏层；游戏层按 tick 推进世界并广播状态。")
	fmt.Println("本演示压缩成 6 个 tick：疾风游侠 fast 和断流骑士 slow 都已进入房间并准备。")
	fmt.Println("改进写法：tick 只消费已经到达的 ACTION；dirtyPlayers 只记录本 tick 变化过的玩家。")
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

	return runEventDrivenLoop(newGameState(), sessions)
}

func runEventDrivenLoop(world gameState, sessions map[string]*session) error {
	actions := make(chan incomingAction, 8)
	dirtyPlayers := map[string]bool{}

	for _, s := range sessions {
		if err := writeLine(s.writer, "START_ACTION"); err != nil {
			return err
		}
		go receiveAction(s, actions)
	}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	start := time.Now()
	fmt.Println("\n游戏开始：服务器按 tick 推进权威世界。")
	fmt.Println("初始世界状态：", world.render())

	for tick := 1; tick <= totalTicks; tick++ {
		<-ticker.C
		fmt.Printf("\nTick %d [%s]\n", tick, formatDuration(time.Since(start)))

		handled := 0
		for {
			/*
				================ 【学生重点 6/6】事件驱动主循环：Warzone 游戏嵌入版 ================

				请只看这个 select/default：
				1. case got := <-actions：本 tick 前已经到达的 ACTION 会被应用到权威 PlayerState。
				2. default：当前没有 ACTION，立刻结束本 tick 的收包阶段，不阻塞等待慢玩家。

				放到 Warzone 这类游戏里能解决什么问题：
				服务器按 tick 推进世界；断流骑士 slow 没来时，世界不暂停，疾风游侠 fast 的移动先同步出去。
				================================================================================
			*/
			select {
			case got := <-actions:
				if got.err != nil {
					return got.err
				}
				world.applyAction(got.event)
				dirtyPlayers[got.event.player] = true
				handled++
				fmt.Printf("[应用 ACTION] %s %-10s 客户端模拟延迟=%dms\n",
					heroName(got.event.player), got.event.action, got.event.delayMS)
			default:
				goto syncDirty
			}
		}

	syncDirty:
		if handled == 0 {
			fmt.Println("没有新 ACTION：服务器不等待，继续下一 tick。")
		}
		if len(dirtyPlayers) == 0 {
			fmt.Println("[增量 STATE_UPDATE] 没有玩家变化，不广播。")
		} else {
			/*
				================ 【学生重点 6/6】增量同步 dirtyPlayers：Warzone 游戏嵌入版 ================

				请只看 dirtyPlayers：
				1. 有玩家位置变化时，前面会执行 dirtyPlayers[player] = true。
				2. 这里同步时只遍历 dirtyPlayers，而不是每个 tick 都广播完整世界。

				放到 Warzone 这类游戏里能解决什么问题：
				只把本 tick 变化过的玩家同步给客户端，减少无变化帧里的网络广播。
				================================================================================
			*/
			fmt.Print("[增量 STATE_UPDATE] 只同步变化玩家：")
			for player := range dirtyPlayers {
				p := world.players[player]
				fmt.Printf("%s=(%d,%d) ", p.name, p.x, p.y)
			}
			fmt.Println()
			clear(dirtyPlayers)
		}
		fmt.Println("当前世界状态：", world.render())
	}

	for _, s := range sessions {
		if err := writeLine(s.writer, "DONE "+world.render()); err != nil {
			return err
		}
	}
	fmt.Println("\n结论：事件驱动让游戏世界持续推进；增量同步只发送发生变化的玩家。")
	return nil
}

func receiveAction(s *session, actions chan<- incomingAction) {
	ev, err := readAction(s.reader)
	actions <- incomingAction{event: ev, err: err}
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
