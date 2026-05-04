package main

import (
	"bufio"
	"ch3/internal/ch3game"
	"ch3/internal/ch3proto"
	"ch3/internal/ch3render"
	"fmt"
	"net"
	"os"
	"strings"
)

func parseInput(s string) ch3game.Input {
	s = strings.TrimSpace(strings.ToLower(s))
	in := ch3game.Input{}
	switch s {
	case "w":
		in.MoveY = -1
	case "s":
		in.MoveY = 1
	case "a":
		in.MoveX = -1
	case "d":
		in.MoveX = 1
	case "j":
		in.Attack = true
	}
	return in
}

func toMsg(frame int, in ch3game.Input) ch3proto.InputMsg {
	action := "idle"
	if in.MoveX == -1 {
		action = "left"
	} else if in.MoveX == 1 {
		action = "right"
	} else if in.MoveY == -1 {
		action = "up"
	} else if in.MoveY == 1 {
		action = "down"
	} else if in.Attack {
		action = "attack"
	}
	return ch3proto.InputMsg{PlayerID: 0, Frame: frame, Action: action}
}

func fromMsg(m ch3proto.InputMsg) ch3game.Input {
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

func main() {
	ln, err := net.Listen("tcp", ":9104")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	fmt.Println("Step4 host listening :9104")
	conn, err := ln.Accept()
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	reader := bufio.NewReader(os.Stdin)
	state := ch3game.State{P0: ch3game.Fighter{X: 1, Y: 1, HP: 100}, P1: ch3game.Fighter{X: 2, Y: 1, HP: 100}}

	for frame := 1; frame <= 8; frame++ {
		/*
			================ 【学生重点 第三章：P2P 锁步帧】 ================
			每一帧必须先交换输入，再计算状态。
			如果对方输入没到，本端就停在 RecvJSON 等待，这正是“锁步”。
			好处是双方都用同一份输入计算；代价是慢的一方会拖住这一帧。
			============================================================
		*/
		fmt.Printf("[host frame %d] 输入(w/a/s/d/j): ", frame)
		line, _ := reader.ReadString('\n')
		local := parseInput(line)
		_ = ch3proto.SendJSON(conn, toMsg(frame, local))

		var rm ch3proto.InputMsg
		if err := ch3proto.RecvJSON(conn, &rm); err != nil {
			fmt.Println("peer disconnected:", err)
			return
		}
		remote := fromMsg(rm)
		state = ch3game.DeterministicUpdate(state, local, remote, true)
		fmt.Printf("state frame=%d p0(%d,%d,hp=%d) p1(%d,%d,hp=%d) event=%s\n", state.Frame, state.P0.X, state.P0.Y, state.P0.HP, state.P1.X, state.P1.Y, state.P1.HP, state.Event)
		fmt.Println(ch3render.RenderDuelMap(state.P0.X, state.P0.Y, state.P1.X, state.P1.Y, 20, 10))
		if state.Over {
			fmt.Println("游戏结束")
			return
		}
	}
}
