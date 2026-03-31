package main

import (
	"ch3/internal/ch3net"
	"ch3/internal/ch3proto"
	"fmt"
	"math"
	"net"
	"sync"
	"time"
)

const (
	maxPlayers        = 2
	mapW              = 20
	mapH              = 10
	tickRate          = 200 * time.Millisecond
	heartbeatAction   = "heartbeat"
	playerIdleTimeout = 3 * time.Second
	recvTimeout       = 500 * time.Millisecond
)

type player struct {
	rc       *ch3net.ReliableConn
	id       int
	x, y     int
	hp       int
	online   bool
	lastSeen time.Time
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

// applyInput 将一帧输入作用到玩家位置（攻击由战斗逻辑统一处理）。
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

// update 推进一帧世界状态：处理输入、结算攻击、清空输入并生成事件文本。
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

// makeSnapshot 复制当前世界状态，供广播阶段在锁外发送。
func makeSnapshot(event string) snapshot {
	ps := make([]ch3proto.PlayerState, len(players))
	for i, p := range players {
		ps[i] = ch3proto.PlayerState{ID: p.id, X: p.x, Y: p.y, HP: p.hp, Online: p.online}
	}
	return snapshot{frame: frame, players: ps, event: event}
}

// attachPlayer 绑定（或重绑）连接到指定玩家 ID，支持断线重连。
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
	p.lastSeen = time.Now()
	return p
}

// recvWorker 持续接收某个玩家的输入，并写入该玩家对应的输入槽位。
func recvWorker(p *player) {
	rc := p.rc
	for {
		var msg ch3proto.InputMsg
		if err := rc.Recv(recvTimeout, &msg); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				mu.Lock()
				if p.rc != rc || !p.online {
					mu.Unlock()
					return
				}
				if time.Since(p.lastSeen) > playerIdleTimeout {
					fmt.Printf("[server] player#%d heartbeat timeout\n", p.id)
					p.online = false
					_ = rc.Close()
					p.rc = nil
					mu.Unlock()
					return
				}
				mu.Unlock()
				continue
			}
			mu.Lock()
			if p.rc == rc {
				fmt.Printf("[server] player#%d 断开: %v\n", p.id, err)
				p.online = false
				p.rc = nil
			}
			mu.Unlock()
			return
		}
		mu.Lock()
		if !p.online || p.rc != rc {
			mu.Unlock()
			return
		}
		p.lastSeen = time.Now()
		if msg.Action == heartbeatAction {
			mu.Unlock()
			continue
		}
		inputs[p.id] = msg
		mu.Unlock()
	}
}

// broadcast 将快照发送给所有在线玩家；发送超时仅丢帧，发送失败则标记离线。
func broadcast(s snapshot) {
	state := ch3proto.WorldState{Frame: s.frame, Players: s.players, Event: s.event}
	type target struct {
		id int
		rc *ch3net.ReliableConn
	}

	mu.Lock()
	targets := make([]target, 0, len(players))
	for _, p := range players {
		if p.online && p.rc != nil {
			targets = append(targets, target{id: p.id, rc: p.rc})
		}
	}
	mu.Unlock()

	for _, t := range targets {
		err := t.rc.SendTimeout(30*time.Millisecond, state)
		if err == nil {
			continue
		}
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			fmt.Printf("[server] player#%d send timeout, drop frame=%d\n", t.id, s.frame)
			continue
		}
		mu.Lock()
		p := players[t.id]
		if p.rc == t.rc {
			fmt.Printf("[server] player#%d send failed: %v\n", p.id, err)
			p.online = false
			_ = p.rc.Close()
			p.rc = nil
		}
		mu.Unlock()
	}
}

// acceptLoop 持续接入新连接，校验 Join 消息并启动对应玩家输入接收协程。
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
			_ = rc.SendTimeout(200*time.Millisecond, ch3proto.WorldState{Event: "服务器满员，无法加入/重连"})
			_ = conn.Close()
			continue
		}
		fmt.Printf("[server] player#%d 已连接/重连，恢复位置(%d,%d) hp=%d\n", p.id, p.x, p.y, p.hp)
		_ = p.rc.SendTimeout(200*time.Millisecond, ch3proto.WorldState{
			Frame:   frame,
			Players: []ch3proto.PlayerState{{ID: p.id, X: p.x, Y: p.y, HP: p.hp, Online: true}},
			Event:   fmt.Sprintf("rejoined as player %d", p.id),
		})
		go recvWorker(p)
	}
}

// main 启动权威服务器：监听连接、按固定 Tick 更新世界并广播状态。
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

	for frame < 10000 {
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
