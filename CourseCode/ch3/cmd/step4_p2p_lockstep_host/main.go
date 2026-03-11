package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"warzone/exp6/internal/exp6game"
	"warzone/exp6/internal/exp6proto"
)

func parseInput(s string) exp6game.Input {
	s = strings.TrimSpace(strings.ToLower(s))
	in := exp6game.Input{}
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

func toMsg(frame int, in exp6game.Input) exp6proto.InputMsg {
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
	return exp6proto.InputMsg{PlayerID: 0, Frame: frame, Action: action}
}

func fromMsg(m exp6proto.InputMsg) exp6game.Input {
	in := exp6game.Input{}
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
	state := exp6game.State{P0: exp6game.Fighter{X: 1, Y: 1, HP: 100}, P1: exp6game.Fighter{X: 2, Y: 1, HP: 100}}

	for frame := 1; frame <= 8; frame++ {
		fmt.Printf("[host frame %d] 输入(w/a/s/d/j): ", frame)
		line, _ := reader.ReadString('\n')
		local := parseInput(line)
		_ = exp6proto.SendJSON(conn, toMsg(frame, local))

		var rm exp6proto.InputMsg
		if err := exp6proto.RecvJSON(conn, &rm); err != nil {
			fmt.Println("peer disconnected:", err)
			return
		}
		remote := fromMsg(rm)
		state = exp6game.DeterministicUpdate(state, local, remote, true)
		fmt.Printf("state frame=%d p0(%d,%d,hp=%d) p1(%d,%d,hp=%d) event=%s\n", state.Frame, state.P0.X, state.P0.Y, state.P0.HP, state.P1.X, state.P1.Y, state.P1.HP, state.Event)
	}
}
