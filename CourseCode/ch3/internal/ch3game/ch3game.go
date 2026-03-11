package ch3game

import "math"

type Input struct {
	MoveX  int
	MoveY  int
	Attack bool
}

type Fighter struct {
	X  int
	Y  int
	HP int
}

type State struct {
	Frame int
	P0    Fighter
	P1    Fighter
	Event string
	Over  bool
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

func DeterministicUpdate(prev State, local Input, remote Input, isHost bool) State {
	next := prev
	next.Frame++
	next.Event = ""

	var in0, in1 Input
	if isHost {
		in0, in1 = local, remote
	} else {
		in0, in1 = remote, local
	}

	next.P0.X = clamp(next.P0.X+in0.MoveX, 0, 20)
	next.P0.Y = clamp(next.P0.Y+in0.MoveY, 0, 10)
	next.P1.X = clamp(next.P1.X+in1.MoveX, 0, 20)
	next.P1.Y = clamp(next.P1.Y+in1.MoveY, 0, 10)

	dx := float64(next.P0.X - next.P1.X)
	dy := float64(next.P0.Y - next.P1.Y)
	dist := math.Sqrt(dx*dx + dy*dy)

	if in0.Attack && dist <= 1.0 {
		next.P1.HP -= 10
		next.Event += "P0 hit P1; "
	}
	if in1.Attack && dist <= 1.0 {
		next.P0.HP -= 10
		next.Event += "P1 hit P0; "
	}
	if next.P0.HP < 0 {
		next.P0.HP = 0
	}
	if next.P1.HP < 0 {
		next.P1.HP = 0
	}
	if next.P0.HP <= 0 || next.P1.HP <= 0 {
		next.Over = true
		if next.P0.HP <= 0 && next.P1.HP <= 0 {
			next.Event += "double KO"
		} else if next.P0.HP <= 0 {
			next.Event += "P1 wins"
		} else {
			next.Event += "P0 wins"
		}
	}
	if next.Event == "" {
		next.Event = "none"
	}
	return next
}
