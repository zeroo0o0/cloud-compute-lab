package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

type gameState struct {
	Tick   int
	X      int
	Y      int
	HP     int
	Status string
}

func readInput(r *bufio.Reader) string {
	fmt.Print("输入(w/a/s/d移动, j攻击, q退出): ")
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(line))
}

func updateState(s *gameState, in string) bool {
	s.Tick++
	s.Status = "idle"
	switch in {
	case "w":
		s.Y--
		s.Status = "move up"
	case "s":
		s.Y++
		s.Status = "move down"
	case "a":
		s.X--
		s.Status = "move left"
	case "d":
		s.X++
		s.Status = "move right"
	case "j":
		s.Status = "attack!"
	case "q":
		return false
	}
	return true
}

func render(s gameState) {
	fmt.Printf("\n[Frame=%d] Pos=(%d,%d) HP=%d Status=%s\n\n", s.Tick, s.X, s.Y, s.HP, s.Status)
}

func main() {
	fmt.Println("=== Step1: 单机本地游戏原型（输入->计算->渲染）===")
	reader := bufio.NewReader(os.Stdin)
	state := gameState{HP: 100}
	for {
		in := readInput(reader)
		if ok := updateState(&state, in); !ok {
			fmt.Println("退出游戏循环")
			return
		}
		render(state)
		time.Sleep(50 * time.Millisecond)
	}
}
