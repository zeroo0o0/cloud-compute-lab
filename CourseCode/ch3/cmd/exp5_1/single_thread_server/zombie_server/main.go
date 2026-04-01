package main

import (
	"ch3/internal/ch3proto"
	"fmt"
	"math"
	"net"
	"time"
)

const (
	maxPlayers = 2
	mapW       = 20
	mapH       = 10
	tickRate   = 200 * time.Millisecond
)

type player struct {
	conn   net.Conn
	id     int
	x, y   int
	hp     int
	online bool
}

var (
	players []*player
	inputs  [maxPlayers]ch3proto.InputMsg
	frame   int
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
	event := ""
	frameInputs := inputs
	for i, p := range players {
		if !p.online {
			continue
		}
		applyInput(p, frameInputs[i])
		inputs[i] = ch3proto.InputMsg{}
	}
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
	if event == "" {
		event = "none"
	}
	return event
}

func makeSnapshot(event string) ch3proto.WorldState {
	ps := make([]ch3proto.PlayerState, len(players))
	for i, p := range players {
		ps[i] = ch3proto.PlayerState{ID: p.id, X: p.x, Y: p.y, HP: p.hp, Online: p.online}
	}
	return ch3proto.WorldState{Frame: frame, Players: ps, Event: event}
}

func broadcast(state ch3proto.WorldState) {
	for _, p := range players {
		if !p.online {
			continue
		}
		_ = ch3proto.SendJSON(p.conn, state)
	}
}

func main() {
	// 阶段1：启动监听并等待客户端连接
	ln, err := net.Listen("tcp", ":9107")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	fmt.Println("=== zombie 单线程服务器(断网演示) :9107 ===")
	fmt.Println("等待 2 个客户端连接...")

	for len(players) < maxPlayers {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		id := len(players)
		p := &player{conn: conn, id: id, x: id * 5, y: 5, hp: 100, online: true}
		players = append(players, p)
		fmt.Printf("[server] player#%d connected from %s\n", id, conn.RemoteAddr())
	}
	fmt.Println("[server] 全员就绪，进入单线程阻塞循环")

	// 阶段2：广播首帧（init），让客户端进入游戏画面
	broadcast(makeSnapshot("init"))

	// 阶段3：单线程主循环（等待输入 -> 计算 -> 广播）
	for frame < 10000 {
		// 阶段3.1：按玩家顺序阻塞读取输入（这是本实验的关键阻塞点）
		for i, p := range players {
			if !p.online {
				inputs[i] = ch3proto.InputMsg{}
				continue
			}
			var msg ch3proto.InputMsg
			if err := ch3proto.RecvJSON(p.conn, &msg); err != nil {
				fmt.Printf("[server] player#%d 断开: %v\n", p.id, err)
				p.online = false
				_ = p.conn.Close()
				inputs[i] = ch3proto.InputMsg{}
				continue
			}
			if msg.Action == "quit" {
				fmt.Printf("[server] player#%d quit\n", p.id)
				p.online = false
				_ = p.conn.Close()
				inputs[i] = ch3proto.InputMsg{}
				continue
			}
			inputs[i] = msg
		}

		// 阶段3.2：用本帧输入推进状态
		event := update()

		// 阶段3.3：向所有在线玩家广播最新状态
		broadcast(makeSnapshot(event))

		// 阶段3.4：固定节奏等待，进入下一帧
		time.Sleep(tickRate)
	}
	fmt.Println("[server] 演示结束 (10000帧)")
}
