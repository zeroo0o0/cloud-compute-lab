package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"warzone/ch3/internal/ch3game"
	"warzone/ch3/internal/ch3proto"
	"warzone/ch3/internal/ch3render"
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
	return ch3proto.InputMsg{PlayerID: 1, Frame: frame, Action: action}
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
	host := "127.0.0.1"
	if len(os.Args) > 1 {
		host = os.Args[1]
	}
	conn, err := net.Dial("tcp", host+":9104")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	reader := bufio.NewReader(os.Stdin)
	state := ch3game.State{P0: ch3game.Fighter{X: 1, Y: 1, HP: 100}, P1: ch3game.Fighter{X: 2, Y: 1, HP: 100}}

	for frame := 1; frame <= 8; frame++ {
		fmt.Printf("[client frame %d] 输入(w/a/s/d/j): ", frame)
		line, _ := reader.ReadString('\n')
		local := parseInput(line)
		_ = ch3proto.SendJSON(conn, toMsg(frame, local))

		var rm ch3proto.InputMsg
		if err := ch3proto.RecvJSON(conn, &rm); err != nil {
			fmt.Println("host disconnected:", err)
			return
		}
		remote := fromMsg(rm)
		state = ch3game.DeterministicUpdate(state, local, remote, false)
		fmt.Printf("state frame=%d p0(%d,%d,hp=%d) p1(%d,%d,hp=%d) event=%s\n", state.Frame, state.P0.X, state.P0.Y, state.P0.HP, state.P1.X, state.P1.Y, state.P1.HP, state.Event)
		fmt.Println(ch3render.RenderDuelMap(state.P0.X, state.P0.Y, state.P1.X, state.P1.Y, 20, 10))
		if state.Over {
			fmt.Println("游戏结束")
			return
		}
	}
}
