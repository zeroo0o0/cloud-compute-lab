package main

import (
	"fmt"
	"math"
	"net"
	"sync"
	"time"
	"warzone/exp6/internal/exp6net"
	"warzone/exp6/internal/exp6proto"
)

const (
	maxPlayers = 2
	mapW       = 20
	mapH       = 10
	tickRate   = 200 * time.Millisecond
)

type player struct {
	rc   *exp6net.ReliableConn
	id   int
	x, y int
	hp   int
}

var (
	mu      sync.Mutex
	players []*player
	inputs  [maxPlayers]exp6proto.InputMsg
	frame   int
)

type snapshot struct {
	frame   int
	players []exp6proto.PlayerState
	event   string
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func recvWorker(p *player) {
	for {
		var msg exp6proto.InputMsg
		// 使用 ReliableConn 的超时 Recv (500ms)
		if err := p.rc.Recv(500*time.Millisecond, &msg); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue // 超时不断开，继续下一轮
			}
			mu.Lock()
			fmt.Printf("[server] player#%d 断开: %v\n", p.id, err)
			p.hp = 0
			mu.Unlock()
			return
		}
		mu.Lock()
		inputs[p.id] = msg
		mu.Unlock()
	}
}

func applyInput(p *player, m exp6proto.InputMsg) {
	switch m.Action {
	case "left":
		p.x = clamp(p.x-1, 0, mapW)
	case "right":
		p.x = clamp(p.x+1, 0, mapW)
	case "up":
		p.y = clamp(p.y-1, 0, mapH)
	case "down":
		p.y = clamp(p.y+1, 0, mapH)
	case "attack":
		// 攻击逻辑在下方
	}
}

func update() string {
	frame++
	event := ""
	frameInputs := inputs
	for i, p := range players {
		applyInput(p, frameInputs[i])
	}
	// 攻击判定
	if len(players) == 2 {
		p0, p1 := players[0], players[1]
		dx := float64(p0.x - p1.x)
		dy := float64(p0.y - p1.y)
		dist := math.Sqrt(dx*dx + dy*dy)
		if frameInputs[0].Action == "attack" && dist < 2 {
			p1.hp -= 10
			event += "P0 hit P1! "
		}
		if frameInputs[1].Action == "attack" && dist < 2 {
			p0.hp -= 10
			event += "P1 hit P0! "
		}
	}
	// 清空本帧输入
	for i := range inputs {
		inputs[i] = exp6proto.InputMsg{}
	}
	if len(players) > 0 && players[0].hp < 0 {
		players[0].hp = 0
	}
	if len(players) > 1 && players[1].hp < 0 {
		players[1].hp = 0
	}
	return event
}

func makeSnapshot(event string) snapshot {
	ps := make([]exp6proto.PlayerState, len(players))
	for i, p := range players {
		ps[i] = exp6proto.PlayerState{ID: p.id, X: p.x, Y: p.y, HP: p.hp}
	}
	return snapshot{frame: frame, players: ps, event: event}
}

func broadcast(s snapshot) {
	state := exp6proto.WorldState{Frame: s.frame, Players: s.players, Event: s.event}
	for _, p := range players {
		_ = p.rc.Send(state)
	}
}

func main() {
	ln, err := net.Listen("tcp", ":9108")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	fmt.Println("=== Step7 ReliableConn 权威服务器 :9108 ===")
	fmt.Println("特点: 使用 ReliableConn 封装 SetReadDeadline 超时机制")
	fmt.Println("即使客户端暂时掉线/延迟，服务器主循环也不会卡死")
	fmt.Println("等待 2 个客户端连接...")

	for len(players) < maxPlayers {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetNoDelay(true)
		}
		id := len(players)
		p := &player{rc: exp6net.NewReliableConn(conn), id: id, x: id * 10, y: 5, hp: 100}
		players = append(players, p)
		fmt.Printf("[server] player#%d connected from %s\n", id, conn.RemoteAddr())
		go recvWorker(p)
	}
	fmt.Println("[server] 全员就绪，权威主循环开始 (tick=200ms, recv timeout=500ms)")

	gameOver := false
	for !gameOver && frame < 200 {
		time.Sleep(tickRate)
		mu.Lock()
		event := update()
		snap := makeSnapshot(event)
		// 检查胜负
		for _, p := range players {
			if p.hp <= 0 {
				event = fmt.Sprintf("Player#%d 被击败!", p.id)
				gameOver = true
			}
		}
		snap.event = event
		if frame%10 == 0 || event != "" {
			for _, p := range players {
				fmt.Printf("  p%d(%d,%d,hp=%d)", p.id, p.x, p.y, p.hp)
			}
			fmt.Printf("  frame=%d event=%s\n", frame, event)
		}
		mu.Unlock()
		broadcast(snap)
	}
	fmt.Println("[server] 游戏结束")
	for _, p := range players {
		p.rc.Close()
	}
}
