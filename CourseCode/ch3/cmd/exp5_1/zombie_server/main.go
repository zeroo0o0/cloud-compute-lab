package main

import (
	"ch3/internal/ch3game"
	"ch3/internal/ch3proto"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

// Step5.1: TCP 半开连接（僵尸玩家）演示（简化：仅保留 BP/pressure 模式）
//
// BP(pressure): 服务器固定 tick 推进；当某客户端进入 blackhole（不收包）后，
//               服务端对其 send 更容易被拖慢/阻塞（发送缓冲区被填满），进而影响正常玩家。
//
// 说明：为了复现现象，本实验刻意不加 read timeout/心跳。

const (
	addr     = ":9110"
	mapW     = 20
	mapH     = 10
	tickRate = 200 * time.Millisecond
)

type player struct {
	id   int
	conn net.Conn

	online bool
	lastIn ch3game.Input
}

func parseMaxFrames(args []string) int {
	// 用法：zombie_server [maxFrames]
	if len(args) >= 2 {
		if v, err := strconv.Atoi(args[1]); err == nil {
			return v
		}
	}
	return 999999
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
	ws := ch3proto.WorldState{
		Frame: s.Frame,
		Players: []ch3proto.PlayerState{
			{ID: 0, X: s.P0.X, Y: s.P0.Y, HP: s.P0.HP, Online: online0},
			{ID: 1, X: s.P1.X, Y: s.P1.Y, HP: s.P1.HP, Online: online1},
		},
		Event: event,
	}
	return ws
}

func broadcastWithPressure(players []*player, ws ch3proto.WorldState) {
	// 假设 player#1 更容易被用来做“僵尸玩家”（课堂上通常让 P1 断网/不读）
	// 做一个更激进的演示：先给 P1 连续发很多次“同一个 WorldState”，增加其 socket 发送缓冲被打满的概率。
	// 如果对端不读，这些写操作可能阻塞（或变慢），从而影响后续给 P0 的广播。
	if len(players) < 2 {
		for _, p := range players {
			_ = ch3proto.SendJSON(p.conn, ws)
		}
		return
	}

	// 先向 P1 施压（顺序刻意：P1 -> P0）
	// tight deadline 避免在某些环境下完全卡死服务器进程。
	for i := 0; i < 96; i++ {
		_ = players[1].conn.SetWriteDeadline(time.Now().Add(25 * time.Millisecond))
		_ = ch3proto.SendJSON(players[1].conn, ws)
	}
	_ = players[1].conn.SetWriteDeadline(time.Time{})

	_ = players[0].conn.SetWriteDeadline(time.Now().Add(800 * time.Millisecond))
	_ = ch3proto.SendJSON(players[0].conn, ws)
	_ = players[0].conn.SetWriteDeadline(time.Time{})
}

func recvLoop(p *player, mu *sync.Mutex, inputs *[2]ch3game.Input, got *[2]bool) {
	for {
		var msg ch3proto.InputMsg
		if err := ch3proto.RecvJSON(p.conn, &msg); err != nil {
			mu.Lock()
			p.online = false
			mu.Unlock()
			return
		}
		in := inputFromMsg(msg)
		mu.Lock()
		p.lastIn = in
		inputs[p.id] = in
		got[p.id] = true
		mu.Unlock()
	}
}

func main() {
	maxFrames := parseMaxFrames(os.Args)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	fmt.Println("=== Step5.1 僵尸玩家 / TCP 半开连接 演示服务器 ===")
	fmt.Println("listen:", addr)
	fmt.Println("mode:", "BP(pressure)")
	fmt.Println("玩法: 双人对战（权威服务器推进）")
	fmt.Println("演示: 让 client1 使用 t 进入 blackhole(不发不收且不关连接)，观察 server 与正常玩家的体感")
	fmt.Println()

	players := make([]*player, 0, 2)
	for len(players) < 2 {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		id := len(players)
		p := &player{id: id, conn: conn, online: true}
		players = append(players, p)
		fmt.Printf("[server] player#%d connected from %s\n", id, conn.RemoteAddr())
	}

	// 初始化局面
	state := ch3game.State{
		Frame: 0,
		P0:    ch3game.Fighter{X: 2, Y: 5, HP: 100},
		P1:    ch3game.Fighter{X: 18, Y: 5, HP: 100},
	}
	_ = mapW
	_ = mapH

	var mu sync.Mutex
	var inputs [2]ch3game.Input
	var got [2]bool

	for _, p := range players {
		go recvLoop(p, &mu, &inputs, &got)
	}

	lastLog := time.Now()
	// 为了避免“帧滚动太快导致刷屏”，本实验改为：
	// - 仍然按 tickRate 轮询一次
	// - 只有当收到任意一方输入（got[0] / got[1]）时才推进并广播
	// 这样客户端只会在用户输入导致状态变化时渲染。
	needSend := true // 首帧必须同步一次初始状态
	for !state.Over && state.Frame < maxFrames {
		time.Sleep(tickRate)
		mu.Lock()
		in0 := inputs[0]
		in1 := inputs[1]
		anyGot := got[0] || got[1]
		inputs[0], inputs[1] = ch3game.Input{}, ch3game.Input{}
		got[0], got[1] = false, false
		on0, on1 := players[0].online, players[1].online
		mu.Unlock()

		// 没有输入就不推进、不广播（避免刷屏）
		if !needSend && !anyGot {
			continue
		}
		needSend = false

		state = ch3game.DeterministicUpdate(state, in0, in1, true)
		event := "BP(pressure): broadcast only on input; pressure on zombie may lag normal"
		ws := worldStateFromGame(state, on0, on1, event)
		broadcastWithPressure(players, ws)

		if time.Since(lastLog) > 1*time.Second {
			fmt.Printf("[server] frame=%d (online0=%v online1=%v)\n", state.Frame, on0, on1)
			lastLog = time.Now()
		}
	}

	fmt.Println("[server] game over:", state.Event)
}
