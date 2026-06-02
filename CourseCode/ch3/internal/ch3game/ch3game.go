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

	/*
		================ 【学生重点 第三章：确定性锁步】 ================
		Host 和 Client 都调用同一个 DeterministicUpdate。
		差别只在于 local/remote 分别对应 P0 还是 P1。
		只要双方收到的是同一帧的两份输入，且计算函数完全一致，
		两台机器就能独立算出同一个世界状态。
		============================================================
	*/
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

	/*
		================ 【学生重点 第三章：规则也必须确定】 ================
		锁步同步的不是坐标结果，而是输入。
		所以命中距离、扣血数值、死亡条件都必须在双方代码里保持一致。
		任何一处规则不一致，都会导致两边状态逐帧漂移。
		================================================================
	*/
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
