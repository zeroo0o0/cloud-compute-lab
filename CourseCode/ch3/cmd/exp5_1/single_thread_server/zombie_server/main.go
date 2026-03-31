package main

import (
	"ch3/internal/ch3game"
	"ch3/internal/ch3proto"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

// Step5.1 单线程读阻塞演示：
// - 只有主循环 goroutine 负责按 tick 轮询读取两个客户端输入，再广播。
// - 读取顺序固定且为阻塞读窗口；某个玩家长时间不发包，会拖慢同一 tick 内后续玩家。

const (
	addr           = ":9110"
	tickRate       = 200 * time.Millisecond
	readPollWindow = 150 * time.Millisecond
	modeRead       = "single-thread-read-block"
	maxPlayers     = 2
)

type player struct {
	id          int
	conn        net.Conn
	online      bool
	lastInputAt time.Time
}

func parseArgs(args []string) int {
	// 用法：zombie_server [maxFrames]
	maxFrames := 999999
	if len(args) >= 2 {
		if v, err := strconv.Atoi(args[1]); err == nil {
			maxFrames = v
		}
	}
	return maxFrames
}

func recvJoinMsg(conn net.Conn, d time.Duration) (int, error) {
	_ = conn.SetReadDeadline(time.Now().Add(d))
	defer conn.SetReadDeadline(time.Time{})

	var j ch3proto.JoinMsg
	if err := ch3proto.RecvJSON(conn, &j); err != nil {
		return -1, err
	}
	if j.PlayerID != 0 && j.PlayerID != 1 {
		return -1, fmt.Errorf("invalid player_id=%d", j.PlayerID)
	}
	return j.PlayerID, nil
}

func inputFromMsg(m ch3proto.InputMsg) ch3game.Input {
	in := ch3game.Input{}
	switch m.Action {
	case "left":
		in.MoveX = -1
	case "right":
		in.MoveX = 1
	case "up":
		in.MoveY = -1
	case "down":
		in.MoveY = 1
	case "attack":
		in.Attack = true
	}
	return in
}

func worldStateFromGame(s ch3game.State, online0, online1 bool, event string) ch3proto.WorldState {
	return ch3proto.WorldState{
		Frame: s.Frame,
		Players: []ch3proto.PlayerState{
			{ID: 0, X: s.P0.X, Y: s.P0.Y, HP: s.P0.HP, Online: online0},
			{ID: 1, X: s.P1.X, Y: s.P1.Y, HP: s.P1.HP, Online: online1},
		},
		Event: event,
	}
}

func bestEffortSend(conn net.Conn, ws ch3proto.WorldState, d time.Duration) error {
	_ = conn.SetWriteDeadline(time.Now().Add(d))
	err := ch3proto.SendJSON(conn, ws)
	_ = conn.SetWriteDeadline(time.Time{})
	return err
}

func rejectConn(conn net.Conn, mode, reason string) {
	ws := ch3proto.WorldState{
		Frame: -1,
		Players: []ch3proto.PlayerState{
			{ID: 0, X: 0, Y: 0, HP: 0, Online: false},
			{ID: 1, X: 0, Y: 0, HP: 0, Online: false},
		},
		Event: reason,
	}
	_ = bestEffortSend(conn, ws, 200*time.Millisecond)
	fmt.Printf("[server] reject client from %s: %s (%s)\n", conn.RemoteAddr(), reason, mode)
	_ = conn.Close()
}

func pollOneInputBlocking(p *player, wait time.Duration) (ch3game.Input, bool, time.Duration, error) {
	start := time.Now()
	_ = p.conn.SetReadDeadline(time.Now().Add(wait))
	defer p.conn.SetReadDeadline(time.Time{})

	var msg ch3proto.InputMsg
	err := ch3proto.RecvJSON(p.conn, &msg)
	cost := time.Since(start)
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return ch3game.Input{}, false, cost, nil
		}
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return ch3game.Input{}, false, cost, nil
		}
		return ch3game.Input{}, false, cost, err
	}
	return inputFromMsg(msg), true, cost, nil
}

func main() {
	maxFrames := parseArgs(os.Args)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	fmt.Println("=== Step5.1 单线程读阻塞演示服务器 ===")
	fmt.Println("listen:", addr)
	fmt.Println("mode:", modeRead)
	fmt.Println("玩法: 双人对战（单线程 ticker 轮询读输入 + 广播）")
	fmt.Println("演示: 让一个客户端按 t 进入 blackhole，观察该玩家阻塞读窗口如何拖慢本 tick 的整体处理")
	fmt.Println()

	players := make([]*player, maxPlayers)
	joined := 0
	for joined < maxPlayers {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}

		requestedID, err := recvJoinMsg(conn, 1200*time.Millisecond)
		if err != nil {
			rejectConn(conn, modeRead, fmt.Sprintf("JOIN FAILED (%s): send JoinMsg{player_id:0|1} after connect", err))
			continue
		}
		if players[requestedID] != nil && players[requestedID].online {
			rejectConn(conn, modeRead, fmt.Sprintf("SLOT OCCUPIED (%s): player_id=%d already connected", modeRead, requestedID))
			continue
		}

		players[requestedID] = &player{id: requestedID, conn: conn, online: true, lastInputAt: time.Now()}
		joined++
		fmt.Printf("[server] player#%d connected from %s (%d/%d)\n", requestedID, conn.RemoteAddr(), joined, maxPlayers)
	}
	fmt.Printf("[server] room slots ready: %d/%d\n", joined, maxPlayers)

	state := ch3game.State{
		Frame: 0,
		P0:    ch3game.Fighter{X: 2, Y: 5, HP: 100},
		P1:    ch3game.Fighter{X: 18, Y: 5, HP: 100},
	}

	firstEvent := "MODE=single-thread-read-block: ticker polls input in one goroutine; idle client can block this tick"
	ws0 := worldStateFromGame(state, players[0].online, players[1].online, firstEvent)
	for _, p := range players {
		_ = bestEffortSend(p.conn, ws0, 300*time.Millisecond)
	}

	lastLog := time.Now()
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()

	for !state.Over && state.Frame < maxFrames {
		<-ticker.C

		var in [2]ch3game.Input
		var got [2]bool
		totalBlock := time.Duration(0)

		for i := 0; i < maxPlayers; i++ {
			p := players[i]
			if p == nil || !p.online {
				continue
			}
			oneIn, oneGot, cost, err := pollOneInputBlocking(p, readPollWindow)
			totalBlock += cost
			if err != nil {
				fmt.Printf("[server][single-thread] player#%d read err: %v\n", p.id, err)
				p.online = false
				continue
			}
			if oneGot {
				in[i] = oneIn
				got[i] = true
				p.lastInputAt = time.Now()
			}
		}

		if got[0] || got[1] {
			state = ch3game.DeterministicUpdate(state, in[0], in[1], true)
		}

		ws := worldStateFromGame(state, players[0].online, players[1].online, "")
		for _, p := range players {
			if p == nil || !p.online {
				continue
			}
			if err := bestEffortSend(p.conn, ws, 120*time.Millisecond); err != nil {
				fmt.Printf("[server][single-thread] player#%d write err: %v\n", p.id, err)
				p.online = false
			}
		}

		if time.Since(lastLog) > time.Second {
			age0 := time.Since(players[0].lastInputAt)
			age1 := time.Since(players[1].lastInputAt)
			fmt.Printf("[server][single-thread] frame=%d block=%.1fms online0=%v online1=%v silence0=%.1fs silence1=%.1fs\n",
				state.Frame,
				float64(totalBlock.Microseconds())/1000.0,
				players[0].online,
				players[1].online,
				age0.Seconds(),
				age1.Seconds(),
			)
			lastLog = time.Now()
		}
	}

	fmt.Println("[server] game over:", state.Event)
}
