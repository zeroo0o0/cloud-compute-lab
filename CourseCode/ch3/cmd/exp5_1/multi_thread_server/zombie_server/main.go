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

// Step5.1: TCP 半开连接（僵尸玩家）演示
//
// read-block: 每个连接在 recv 协程中阻塞读；僵尸连接会长期占住槽位，但不阻塞主循环。
//
// 说明：为了复现现象，本实验刻意不加心跳/read timeout。

const (
	addr       = ":9110"
	mapW       = 20
	mapH       = 10
	tickRate   = 200 * time.Millisecond
	modeRead   = "read-block"
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

// parseArgs 解析服务端启动参数，返回最大帧数。
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

// inputFromMsg 将网络输入消息转换为游戏输入结构。
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

// worldStateFromGame 把内部游戏状态组装成可广播的协议层世界状态。
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

// bestEffortSend 在限定写超时下发送一帧状态，用于常规广播与快速失败。
func bestEffortSend(conn net.Conn, ws ch3proto.WorldState, d time.Duration) error {
	_ = conn.SetWriteDeadline(time.Now().Add(d))
	err := ch3proto.SendJSON(conn, ws)
	_ = conn.SetWriteDeadline(time.Time{})
	return err
}

// handleExtraOrReplacementLoop 处理额外连接：离线槽位复用，否则返回房间已满。
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

		requestedID, err := recvJoinMsg(conn, 1200*time.Millisecond)
		if err != nil {
			rejectConn(conn, mode, fmt.Sprintf("JOIN FAILED (%s): send JoinMsg{player_id:0|1} after connect", err))
			continue
		}

		mu.Lock()
		replaceID := -1
		if players[requestedID] == nil || !players[requestedID].online {
			replaceID = requestedID
		}
		if replaceID >= 0 {
			p := &player{id: replaceID, conn: conn, online: true, assumedUp: true, lastInputAt: time.Now()}
			players[replaceID] = p
			inputs[replaceID] = ch3game.Input{}
			got[replaceID] = false
			mu.Unlock()

			go recvLoop(p, mu, inputs, got)
			fmt.Printf("[server] player#%d joined from %s (mode=%s, requested slot reused)\n", replaceID, conn.RemoteAddr(), mode)
			continue
		}
		mu.Unlock()
		rejectConn(conn, mode, fmt.Sprintf("SLOT OCCUPIED (%s): player_id=%d is online", mode, requestedID))
	}
}

// recvLoop 持续接收单个玩家输入；连接断开时标记该玩家离线。
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

// runReadBlock 运行“读阻塞”模式：展示僵尸连接占槽但不阻塞主循环的现象。
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
		ws := worldStateFromGame(state, on0, on1, "")

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

// main 启动 Step5.1 演示服务器：接收双人连接并根据模式运行对应实验循环。
func main() {
	maxFrames := parseArgs(os.Args)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	fmt.Println("=== Step5.1 僵尸玩家 / TCP 半开连接 演示服务器 ===")
	fmt.Println("listen:", addr)
	fmt.Println("mode:", modeRead)
	fmt.Println("玩法: 双人对战（权威服务器推进）")
	fmt.Println("演示: 让 client1 使用 t 进入 blackhole(不发不收且不关连接)，观察正常玩家与服务器日志")
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

		p := &player{id: requestedID, conn: conn, online: true, assumedUp: true}
		players[requestedID] = p
		joined++
		fmt.Printf("[server] player#%d connected from %s (%d/%d)\n", requestedID, conn.RemoteAddr(), joined, maxPlayers)
	}
	fmt.Printf("[server] room slots ready: %d/%d (offline requested slot can be reused by new client)\n", joined, maxPlayers)

	// 初始化局面
	state := ch3game.State{
		Frame: 0,
		P0:    ch3game.Fighter{X: 2, Y: 5, HP: 100},
		P1:    ch3game.Fighter{X: 18, Y: 5, HP: 100},
	}
	_ = mapW
	_ = mapH

	runReadBlock(ln, modeRead, players, state, maxFrames)
}
