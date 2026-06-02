package main

import (
	"bufio"
	"ch3/internal/ch3render"
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

const (
	mapW = 10
	mapH = 6
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

func readInput(r *bufio.Reader) string {
	fmt.Print("输入(w/a/s/d移动, j攻击, q退出): ")
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(line))
}

func updateState(s *gameState, in string) bool {
	/*
		================ 【学生重点 第三章：游戏主循环】 ================
		这一版还没有网络，只有“输入 -> 更新状态 -> 渲染画面”。
		后续 P2P、C/S 和权威服务器实验，本质上都是在拆分这三个步骤：
		谁收输入、谁计算状态、谁负责渲染。
		============================================================
	*/
	s.Tick++
	s.Status = "idle"
	oldX, oldY := s.X, s.Y
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

	// 限制移动范围：不允许负坐标，且不超过地图边界。
	s.X = clamp(s.X, 0, mapW)
	s.Y = clamp(s.Y, 0, mapH)
	if in == "w" || in == "a" || in == "s" || in == "d" {
		if s.X == oldX && s.Y == oldY {
			s.Status = "blocked"
		}
	}
	return true
}

func render(s gameState) {
	fmt.Printf("\n[Frame=%d] Pos=(%d,%d) HP=%d Status=%s\n", s.Tick, s.X, s.Y, s.HP, s.Status)
	// Step1 只有本地玩家：复用 internal/ch3render 的地图渲染函数。
	fmt.Println(ch3render.RenderDuelMap(s.X, s.Y, -999, -999, mapW, mapH))
	fmt.Println()
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
