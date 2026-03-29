package main

import (
	"ch3/internal/ch3game"
	"ch3/internal/ch3proto"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Step5.1: TCP 半开连接（僵尸玩家）演示
//
// read-block: 每个连接在 recv 协程中阻塞读；僵尸连接会长期占住槽位，但不阻塞主循环。
// write-illusion: 服务端固定 tick 广播，写成功即判在线；可演示“写成功 != 对端存活”。
//
// 说明：为了复现现象，本实验刻意不加心跳/read timeout。

const (
	addr       = ":9110"
	mapW       = 20
	mapH       = 10
	tickRate   = 200 * time.Millisecond
	modeRead   = "read-block"
	modeWrite  = "write-illusion"
	maxPlayers = 2
)

type player struct {
	id          int
	conn        net.Conn
	online      bool
	assumedUp   bool
	lastIn      ch3game.Input
	lastInputAt time.Time
}

func parseArgs(args []string) (string, int) {
	// 用法：zombie_server [mode] [maxFrames]
	mode := modeRead
	maxFrames := 999999
	if len(args) >= 2 {
		m := strings.ToLower(strings.TrimSpace(args[1]))
		if m == modeRead || m == modeWrite {
			mode = m
		} else if v, err := strconv.Atoi(args[1]); err == nil {
			maxFrames = v
		}
	}
	if len(args) >= 3 {
		if v, err := strconv.Atoi(args[2]); err == nil {
			maxFrames = v
		}
	}
	return mode, maxFrames
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

func bestEffortSend(conn net.Conn, ws ch3proto.WorldState, d time.Duration) error {
	_ = conn.SetWriteDeadline(time.Now().Add(d))
	err := ch3proto.SendJSON(conn, ws)
	_ = conn.SetWriteDeadline(time.Time{})
	return err
}

func handleExtraOrReplacementLoop(
	ln net.Listener,
	mode string,
	players []*player,
	mu *sync.Mutex,
	inputs *[2]ch3game.Input,
	got *[2]bool,
) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}

		mu.Lock()
		replaceID := -1
		for i := 0; i < maxPlayers; i++ {
			if players[i] == nil || !players[i].online {
				replaceID = i
				break
			}
		}
		if replaceID >= 0 {
			p := &player{id: replaceID, conn: conn, online: true, assumedUp: true, lastInputAt: time.Now()}
			players[replaceID] = p
			inputs[replaceID] = ch3game.Input{}
			got[replaceID] = false
			mu.Unlock()

			go recvLoop(p, mu, inputs, got)
			fmt.Printf("[server] player#%d joined from %s (mode=%s, slot reused)\n", replaceID, conn.RemoteAddr(), mode)
			continue
		}
		mu.Unlock()

		ws := ch3proto.WorldState{
			Frame: -1,
			Players: []ch3proto.PlayerState{
				{ID: 0, X: 0, Y: 0, HP: 0, Online: false},
				{ID: 1, X: 0, Y: 0, HP: 0, Online: false},
			},
			Event: fmt.Sprintf("ROOM FULL (%s): 2 slots occupied, reconnect later", mode),
		}
		_ = bestEffortSend(conn, ws, 200*time.Millisecond)
		fmt.Printf("[server] reject extra client from %s: room full (%s)\n", conn.RemoteAddr(), mode)
		_ = conn.Close()
	}
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
		p.lastInputAt = time.Now()
		inputs[p.id] = in
		got[p.id] = true
		mu.Unlock()
	}
}

func runReadBlock(ln net.Listener, mode string, players []*player, state ch3game.State, maxFrames int) {
	var mu sync.Mutex
	var inputs [2]ch3game.Input
	var got [2]bool
	for _, p := range players {
		p.lastInputAt = time.Now()
		go recvLoop(p, &mu, &inputs, &got)
	}
	go handleExtraOrReplacementLoop(ln, mode, players, &mu, &inputs, &got)

	// 首帧先同步一次，让客户端能看到模式说明。
	firstEvent := "MODE=read-block: maxConn=2, recv goroutine may block forever and hold slot"
	ws0 := worldStateFromGame(state, players[0].online, players[1].online, firstEvent)
	for _, p := range players {
		_ = bestEffortSend(p.conn, ws0, 300*time.Millisecond)
	}

	lastLog := time.Now()
	for !state.Over && state.Frame < maxFrames {
		time.Sleep(tickRate)
		mu.Lock()
		in0 := ch3game.Input{}
		in1 := ch3game.Input{}
		g0 := got[0]
		g1 := got[1]
		if g0 {
			in0 = inputs[0]
		}
		if g1 {
			in1 = inputs[1]
		}
		inputs[0], inputs[1] = ch3game.Input{}, ch3game.Input{}
		got[0], got[1] = false, false
		on0, on1 := players[0].online, players[1].online
		age0 := time.Since(players[0].lastInputAt)
		age1 := time.Since(players[1].lastInputAt)
		mu.Unlock()

		if !g0 && !g1 {
			if time.Since(lastLog) > 1*time.Second {
				fmt.Printf("[server][read-block] no fresh input (p0 silence=%.1fs p1 silence=%.1fs), slots still occupied\n", age0.Seconds(), age1.Seconds())
				lastLog = time.Now()
			}
			continue
		}

		state = ch3game.DeterministicUpdate(state, in0, in1, true)
		event := fmt.Sprintf("READ-BLOCK(slot-hold): p1 may block on recv and keep slot; third player denied | input-silence p0=%.1fs p1=%.1fs", age0.Seconds(), age1.Seconds())
		ws := worldStateFromGame(state, on0, on1, event)

		// 先发给正常玩家，再尝试给僵尸玩家，避免僵尸写阻塞影响正常玩家。
		_ = bestEffortSend(players[0].conn, ws, 500*time.Millisecond)
		_ = bestEffortSend(players[1].conn, ws, 15*time.Millisecond)

		if time.Since(lastLog) > 1*time.Second {
			fmt.Printf("[server][read-block] frame=%d (online0=%v online1=%v, g0=%v g1=%v)\n", state.Frame, on0, on1, g0, g1)
			lastLog = time.Now()
		}
	}
	fmt.Println("[server] game over:", state.Event)
}

func runWriteIllusion(ln net.Listener, mode string, players []*player, state ch3game.State, maxFrames int) {
	var mu sync.Mutex
	var inputs [2]ch3game.Input
	var got [2]bool

	if tcp, ok := players[1].conn.(*net.TCPConn); ok {
		// 缩小发送缓冲区，更容易复现“写成功一段时间后被塞满”的过程。
		_ = tcp.SetWriteBuffer(4 * 1024)
		_ = tcp.SetNoDelay(true)
	}

	for _, p := range players {
		p.assumedUp = true
		p.lastInputAt = time.Now()
		go recvLoop(p, &mu, &inputs, &got)
	}
	go handleExtraOrReplacementLoop(ln, mode, players, &mu, &inputs, &got)

	lastLog := time.Now()
	for !state.Over && state.Frame < maxFrames {
		time.Sleep(tickRate)
		mu.Lock()
		in0 := ch3game.Input{}
		in1 := ch3game.Input{}
		on0, on1 := players[0].assumedUp, players[1].assumedUp
		age0 := time.Since(players[0].lastInputAt)
		age1 := time.Since(players[1].lastInputAt)
		hasInput := false
		if got[0] {
			in0 = inputs[0]
			hasInput = true
		}
		if got[1] {
			in1 = inputs[1]
			hasInput = true
		}
		inputs[0], inputs[1] = ch3game.Input{}, ch3game.Input{}
		got[0], got[1] = false, false
		mu.Unlock()

		// 当 P1 长时间沉默时，判定为“疑似僵尸连接”，进入写欺骗压测。
		zombieSuspect := on1 && age1 > 1200*time.Millisecond

		// 无输入且无僵尸压力时，不推进帧，避免刷屏。
		if !hasInput && !zombieSuspect {
			if time.Since(lastLog) > 1*time.Second {
				fmt.Printf("[server][write-illusion] idle: p0 silence=%.1fs p1 silence=%.1fs\n", age0.Seconds(), age1.Seconds())
				lastLog = time.Now()
			}
			continue
		}

		state = ch3game.DeterministicUpdate(state, in0, in1, true)
		event := fmt.Sprintf("WRITE-ILLUSION: suspect=%v send() may look OK while zombie unplugged; p1 silence=%.1fs", zombieSuspect, age1.Seconds())
		ws := worldStateFromGame(state, on0, on1, event)

		zombieOK := 0
		zombieErr := 0

		if zombieSuspect {
			// 先对 P1 高压发送，制造其发送缓冲区占满，拖慢后续给 P0 的发送。
			for i := 0; i < 96; i++ {
				err := bestEffortSend(players[1].conn, ws, 40*time.Millisecond)
				if err != nil {
					zombieErr++
					break
				}
				zombieOK++
			}
			_ = bestEffortSend(players[0].conn, ws, 700*time.Millisecond)
		} else {
			_ = bestEffortSend(players[0].conn, ws, 120*time.Millisecond)
			_ = bestEffortSend(players[1].conn, ws, 120*time.Millisecond)
		}

		mu.Lock()
		players[0].assumedUp = players[0].online
		if players[1].online {
			// 演示重点：没有心跳时，P1 即使长期沉默也仍被判在线。
			players[1].assumedUp = true
		} else {
			players[1].assumedUp = false
		}
		mu.Unlock()

		if zombieSuspect && zombieErr > 0 {
			fmt.Printf("[server][write-illusion] zombie send pressure: ok=%d err=%d\n", zombieOK, zombieErr)
		}

		if time.Since(lastLog) > 1*time.Second {
			mu.Lock()
			fmt.Printf("[server][write-illusion] frame=%d p0(online=%v,assumed=%v,silence=%.1fs) p1(online=%v,assumed=%v,silence=%.1fs) buf=4KB\n",
				state.Frame,
				players[0].online, players[0].assumedUp, age0.Seconds(),
				players[1].online, players[1].assumedUp, age1.Seconds(),
			)
			mu.Unlock()
			lastLog = time.Now()
		}
	}
	fmt.Println("[server] game over:", state.Event)
}

func main() {
	mode, maxFrames := parseArgs(os.Args)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	fmt.Println("=== Step5.1 僵尸玩家 / TCP 半开连接 演示服务器 ===")
	fmt.Println("listen:", addr)
	fmt.Println("mode:", mode)
	fmt.Println("玩法: 双人对战（权威服务器推进）")
	fmt.Println("演示: 让 client1 使用 t 进入 blackhole(不发不收且不关连接)，观察正常玩家与服务器日志")
	fmt.Println("可选模式: read-block / write-illusion")
	fmt.Println()

	players := make([]*player, 0, 2)
	for len(players) < maxPlayers {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		id := len(players)
		p := &player{id: id, conn: conn, online: true}
		players = append(players, p)
		fmt.Printf("[server] player#%d connected from %s\n", id, conn.RemoteAddr())
	}
	fmt.Printf("[server] room slots ready: %d/%d (offline slot can be reused by new client)\n", len(players), maxPlayers)

	// 初始化局面
	state := ch3game.State{
		Frame: 0,
		P0:    ch3game.Fighter{X: 2, Y: 5, HP: 100},
		P1:    ch3game.Fighter{X: 18, Y: 5, HP: 100},
	}
	_ = mapW
	_ = mapH

	if mode == modeWrite {
		runWriteIllusion(ln, mode, players, state, maxFrames)
		return
	}
	runReadBlock(ln, mode, players, state, maxFrames)
}
