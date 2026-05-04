package main

import (
	"ch3/internal/ch3net"
	"ch3/internal/ch3proto"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"time"
)

const (
	maxPlayers   = 2
	mapW         = 20
	mapH         = 10
	recvTimeout  = 120 * time.Millisecond
	idleSleep    = 10 * time.Millisecond
	quickTimeout = 5 * time.Millisecond
)

type player struct {
	conn       net.Conn
	rc         *ch3net.ReliableConn
	id         int
	x, y       int
	hp         int
	online     bool
	stickyDemo bool
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
		if p.stickyDemo {
			if err := sendRawBurst(p.conn, state, 3); err != nil {
				fmt.Printf("[server] raw send to player#%d failed: %v\n", p.id, err)
				p.online = false
				_ = p.conn.Close()
			}
			continue
		}
		if err := p.rc.Send(state); err != nil {
			fmt.Printf("[server] send to player#%d failed: %v\n", p.id, err)
			p.online = false
			_ = p.conn.Close()
		}
	}
}

func sendRawBurst(conn net.Conn, state ch3proto.WorldState, count int) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	buf := make([]byte, 0, len(b)*count)
	for i := 0; i < count; i++ {
		buf = append(buf, b...)
	}
	return writeAll(conn, buf)
}

func writeAll(conn net.Conn, b []byte) error {
	for len(b) > 0 {
		n, err := conn.Write(b)
		if n > 0 {
			b = b[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("short write")
		}
	}
	return nil
}

func main() {
	ln, err := net.Listen("tcp", ":9108")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	tcpLn, _ := ln.(*net.TCPListener)
	fmt.Println("=== reliable 单线程服务器(超时读) :9108 ===")
	fmt.Println("等待 2 个客户端连接...")

	for len(players) < maxPlayers {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		id := len(players)
		p := &player{conn: conn, rc: ch3net.NewReliableConn(conn), id: id, x: id * 5, y: 5, hp: 100, online: true}
		players = append(players, p)
		fmt.Printf("[server] player#%d connected from %s\n", id, conn.RemoteAddr())
	}
	fmt.Println("[server] 全员就绪，进入单线程循环 (recv timeout)")

	for frame < 10000 {
		if tcpLn != nil {
			acceptReconnect(tcpLn)
		}
		receivedAny := false
		for i, p := range players {
			if !p.online {
				inputs[i] = ch3proto.InputMsg{}
				continue
			}
			var msg ch3proto.InputMsg
			timeout := recvTimeout
			if receivedAny {
				timeout = quickTimeout
			}
			/*
				================ 【学生重点 第三章：僵尸连接治理】 ================
				单线程服务器仍然按玩家顺序读输入，但这里的 Recv 带超时。
				某个玩家断网或不发包时，本帧把他当作 idle，主循环继续读下一个玩家。
				这正好和 exp5_1 的阻塞读版本形成对照。
				============================================================
			*/
			err := p.rc.Recv(timeout, &msg)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					inputs[i] = ch3proto.InputMsg{Action: "idle"}
					continue
				}
				fmt.Printf("[server] player#%d 断开: %v\n", p.id, err)
				p.online = false
				p.stickyDemo = false
				_ = p.conn.Close()
				inputs[i] = ch3proto.InputMsg{}
				continue
			}
			if msg.Action == "quit" {
				fmt.Printf("[server] player#%d quit\n", p.id)
				p.online = false
				p.stickyDemo = false
				_ = p.conn.Close()
				inputs[i] = ch3proto.InputMsg{}
				continue
			}
			if msg.Action == "p_on" {
				p.stickyDemo = true
				receivedAny = true
				inputs[i] = ch3proto.InputMsg{Action: "idle"}
				continue
			}
			if msg.Action == "p_off" {
				p.stickyDemo = false
				receivedAny = true
				inputs[i] = ch3proto.InputMsg{Action: "idle"}
				continue
			}
			receivedAny = true
			inputs[i] = msg
		}
		if !receivedAny {
			time.Sleep(idleSleep)
			continue
		}
		event := update()
		broadcast(makeSnapshot(event))
	}
	fmt.Println("[server] 演示结束 (10000帧)")
}

func acceptReconnect(tcpLn *net.TCPListener) {
	if len(players) < maxPlayers {
		return
	}
	offlineIdx := -1
	for i, p := range players {
		if !p.online {
			offlineIdx = i
			break
		}
	}
	if offlineIdx == -1 {
		return
	}
	/*
		================ 【学生重点 第三章：重连只恢复连接】 ================
		这里把离线玩家的新 TCP 连接接回原来的 player slot。
		它解决的是“连接断了以后还能回来”，但完整生产系统还需要会话认证、
		状态快照补发、历史输入处理等机制。
		============================================================
	*/
	_ = tcpLn.SetDeadline(time.Now().Add(5 * time.Millisecond))
	conn, err := tcpLn.Accept()
	if err != nil {
		return
	}
	p := players[offlineIdx]
	p.conn = conn
	p.rc = ch3net.NewReliableConn(conn)
	p.online = true
	p.stickyDemo = false
	fmt.Printf("[server] player#%d reconnected from %s\n", p.id, conn.RemoteAddr())
}
