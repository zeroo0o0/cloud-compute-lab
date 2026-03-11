package main

import (
	"fmt"
	"math"
	"net"
	"sync"
	"time"
	"warzone/exp6/internal/exp6proto"
)

const (
	maxPlayers = 2
	mapW       = 20
	mapH       = 10
	tickRate   = 200 * time.Millisecond
)

type player struct {
	conn net.Conn
	id   int
	x, y int
	hp   int
}

var (
	mu      sync.Mutex
	players []*player
	inputs  [maxPlayers]exp6proto.InputMsg // 本帧收集到的输入
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

// recvWorker 持续收取某个客户端的输入
func recvWorker(p *player) {
	for {
		var msg exp6proto.InputMsg
		if err := exp6proto.RecvJSON(p.conn, &msg); err != nil {
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

// applyInput 应用单个输入
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
	}
}

// update 执行权威计算
func update() string {
	frame++
	event := ""
	frameInputs := inputs
	for i, p := range players {
		applyInput(p, frameInputs[i])
		inputs[i] = exp6proto.InputMsg{} // 清空
	}
	// 攻击判定
	if len(players) == 2 {
		p0, p1 := players[0], players[1]
		dx := float64(p0.x - p1.x)
		dy := float64(p0.y - p1.y)
		dist := math.Sqrt(dx*dx + dy*dy)
		if frameInputs[0].Action == "attack" && dist < 2 {
			p1.hp -= 10
			event += fmt.Sprintf("P0 hit P1! dist=%.1f ", dist)
		}
		if frameInputs[1].Action == "attack" && dist < 2 {
			p0.hp -= 10
			event += fmt.Sprintf("P1 hit P0! dist=%.1f ", dist)
		}
		if event == "" && dist < 2 {
			event = fmt.Sprintf("近身! dist=%.1f", dist)
		}
	}
	if p0 := players[0]; p0.hp < 0 {
		p0.hp = 0
	}
	if len(players) > 1 {
		if p1 := players[1]; p1.hp < 0 {
			p1.hp = 0
		}
	}
	return event
}

func makeSnapshot(event string) snapshot {
	ps := make([]exp6proto.PlayerState, len(players))
	for i, p := range players {
		ps[i] = exp6proto.PlayerState{ID: p.id, X: p.x, Y: p.y, HP: p.hp}
	}
	return snapshot{
		frame:   frame,
		players: ps,
		event:   event,
	}
}

// broadcast 将权威状态发给所有客户端。
// 发送网络数据不持有 mu，避免 recvWorker 因 I/O 被长时间阻塞。
func broadcast(s snapshot) {
	state := exp6proto.WorldState{Frame: s.frame, Players: s.players, Event: s.event}
	for _, p := range players {
		_ = exp6proto.SendJSON(p.conn, state)
	}
}

func main() {
	ln, err := net.Listen("tcp", ":9107")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	fmt.Println("=== Step6 权威服务器 :9107 ===")
	fmt.Println("等待 2 个客户端连接...")

	for len(players) < maxPlayers {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		id := len(players)
		p := &player{conn: conn, id: id, x: id * 5, y: 5, hp: 100}
		players = append(players, p)
		fmt.Printf("[server] player#%d connected from %s\n", id, conn.RemoteAddr())
		go recvWorker(p)
	}
	fmt.Println("[server] 全员就绪，开始权威主循环 (tick=200ms)")

	for frame < 500 {
		time.Sleep(tickRate)
		mu.Lock()
		event := update()
		snap := makeSnapshot(event)
		if frame%5 == 0 {
			for _, p := range players {
				fmt.Printf("  p%d(%d,%d,hp=%d)", p.id, p.x, p.y, p.hp)
			}
			fmt.Printf("  frame=%d event=%s\n", frame, event)
		}
		mu.Unlock()
		broadcast(snap)
	}
	fmt.Println("[server] 演示结束 (500帧)")
}
