package main

import (
	"fmt"
	"math"
	"net"
	"sync"
	"time"
	"warzone/ch3/internal/ch3net"
	"warzone/ch3/internal/ch3proto"
)

const (
	maxPlayers = 2
	mapW       = 20
	mapH       = 10
	tickRate   = 200 * time.Millisecond
)

type player struct {
	rc     *ch3net.ReliableConn
	id     int
	x, y   int
	hp     int
	online bool
}

type snapshot struct {
	frame   int
	players []ch3proto.PlayerState
	event   string
}

var (
	mu      sync.Mutex
	players = []*player{
		{id: 0, x: 1, y: 5, hp: 100, online: false},
		{id: 1, x: 18, y: 5, hp: 100, online: false},
	}
	inputs [maxPlayers]ch3proto.InputMsg
	frame  int
)

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func attachPlayer(conn net.Conn, requestedID int) *player {
	mu.Lock()
	defer mu.Unlock()

	if requestedID < 0 || requestedID >= len(players) {
		return nil
	}
	p := players[requestedID]
	if p.online && p.rc != nil {
		_ = p.rc.Close()
	}
	p.rc = ch3net.NewReliableConn(conn)
	p.online = true
	return p
}

func recvWorker(p *player) {
	for {
		var msg ch3proto.InputMsg
		if err := p.rc.Recv(500*time.Millisecond, &msg); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			mu.Lock()
			fmt.Printf("[server] player#%d 断开: %v\n", p.id, err)
			p.online = false
			p.rc = nil
			mu.Unlock()
			return
		}
		mu.Lock()
		if !p.online {
			mu.Unlock()
			continue
		}
		inputs[p.id] = msg
		mu.Unlock()
	}
}

func applyInput(p *player, m ch3proto.InputMsg) {
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

func update() string {
	frame++
	frameInputs := inputs
	event := ""

	for i, p := range players {
		if !p.online {
			continue
		}
		applyInput(p, frameInputs[i])
	}

	if len(players) == 2 && players[0].online && players[1].online {
		p0, p1 := players[0], players[1]
		dx := float64(p0.x - p1.x)
		dy := float64(p0.y - p1.y)
		dist := math.Sqrt(dx*dx + dy*dy)
		if frameInputs[0].Action == "attack" && dist <= 1 {
			p1.hp -= 10
			event += "P0 hit P1! "
		}
		if frameInputs[1].Action == "attack" && dist <= 1 {
			p0.hp -= 10
			event += "P1 hit P0! "
		}
	}

	for i := range inputs {
		inputs[i] = ch3proto.InputMsg{}
	}
	for _, p := range players {
		if p.hp < 0 {
			p.hp = 0
		}
	}
	if !players[0].online || !players[1].online {
		if event == "" {
			event = "有客户端离线，可稍后重连并恢复状态"
		}
	}
	return event
}

func makeSnapshot(event string) snapshot {
	ps := make([]ch3proto.PlayerState, len(players))
	for i, p := range players {
		ps[i] = ch3proto.PlayerState{ID: p.id, X: p.x, Y: p.y, HP: p.hp, Online: p.online}
	}
	return snapshot{frame: frame, players: ps, event: event}
}

func broadcast(s snapshot) {
	state := ch3proto.WorldState{Frame: s.frame, Players: s.players, Event: s.event}
	for _, p := range players {
		if p.online && p.rc != nil {
			_ = p.rc.Send(state)
		}
	}
}

func acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		if tc, ok := conn.(*net.TCPConn); ok {
			_ = tc.SetNoDelay(true)
		}
		rc := ch3net.NewReliableConn(conn)
		var join ch3proto.JoinMsg
		if err := rc.Recv(2*time.Second, &join); err != nil {
			_ = conn.Close()
			continue
		}
		p := attachPlayer(conn, join.PlayerID)
		if p == nil {
			_ = rc.Send(ch3proto.WorldState{Event: "服务器满员，无法加入/重连"})
			_ = conn.Close()
			continue
		}
		fmt.Printf("[server] player#%d 已连接/重连，恢复位置(%d,%d) hp=%d\n", p.id, p.x, p.y, p.hp)
		_ = p.rc.Send(ch3proto.WorldState{
			Frame:   frame,
			Players: []ch3proto.PlayerState{{ID: p.id, X: p.x, Y: p.y, HP: p.hp, Online: true}},
			Event:   fmt.Sprintf("rejoined as player %d", p.id),
		})
		go recvWorker(p)
	}
}

func main() {
	ln, err := net.Listen("tcp", ":9108")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	fmt.Println("=== Step7 ReliableConn 权威服务器 :9108 ===")
	fmt.Println("特点: 使用 ReliableConn 封装 SetReadDeadline 超时机制，并支持重连恢复状态")
	fmt.Println("等待客户端连接/重连...")
	go acceptLoop(ln)

	for frame < 500 {
		time.Sleep(tickRate)
		mu.Lock()
		event := update()
		snap := makeSnapshot(event)
		if frame%5 == 0 || event != "" {
			for _, p := range players {
				fmt.Printf("  p%d(%d,%d,hp=%d,online=%v)", p.id, p.x, p.y, p.hp, p.online)
			}
			fmt.Printf("  frame=%d event=%s\n", frame, event)
		}
		mu.Unlock()
		broadcast(snap)
	}
}
