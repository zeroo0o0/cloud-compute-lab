package main

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	sReset  = "\033[0m"
	sBold   = "\033[1m"
	sDim    = "\033[2m"
	sRed    = "\033[91m"
	sGreen  = "\033[92m"
	sYellow = "\033[93m"
	sCyan   = "\033[96m"
	sWhite  = "\033[97m"
	sBgBlue = "\033[44m"
	sCls    = "\033[2J\033[H"
)

type playerSnapshot struct {
	HP     int
	Online bool
	Alive  bool
}

type serverView struct {
	mu       sync.Mutex
	phase    string
	player1  playerSnapshot
	player2  playerSnapshot
	ghost    bool
	conflict bool
	logs     []string
}

func main() {
	view := &serverView{
		phase:   "等待连接",
		player1: playerSnapshot{HP: 100, Alive: true},
		player2: playerSnapshot{HP: 100, Alive: true},
	}
	view.push("开始监听 127.0.0.1:8888")

	go view.renderLoop()

	listener, err := net.Listen("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	conn1, err := listener.Accept()
	if err != nil {
		panic(err)
	}
	view.mu.Lock()
	view.phase = "玩家1已接入"
	view.player1.Online = true
	view.mu.Unlock()
	view.push("玩家1 已连接")

	conn2, err := listener.Accept()
	if err != nil {
		panic(err)
	}
	view.mu.Lock()
	view.phase = "玩家2已上线"
	view.player2.Online = true
	view.mu.Unlock()
	view.push("玩家2 已连接")

	if _, err = conn1.Write([]byte("P2_ONLINE")); err == nil {
		view.push("已通知玩家1开始攻击")
	}

	buf := make([]byte, 64)
	if _, err = conn1.Read(buf); err != nil {
		view.push("读取攻击指令失败")
		return
	}

	view.mu.Lock()
	view.phase = "服务器判定死亡"
	view.player2.HP = 0
	view.player2.Alive = false
	view.mu.Unlock()
	view.push("收到 ATTACK，玩家2 HP 归零")

	time.Sleep(time.Second)

	msg := []byte("STATE: Player2 DEAD    ")
	_, err1 := conn1.Write(msg)
	_, err2 := conn2.Write(msg)

	view.mu.Lock()
	view.phase = "广播死亡结果"
	view.ghost = err2 == nil
	view.mu.Unlock()
	if err1 == nil {
		view.push("玩家1 收到死亡广播")
	}
	if err2 == nil {
		view.push("旧连接写入仍显示成功")
	} else {
		view.push("旧连接写入失败")
	}

	for {
		conn3, err := listener.Accept()
		if err != nil {
			continue
		}

		view.mu.Lock()
		view.phase = "检测到重连"
		view.player2.Online = true
		view.conflict = true
		view.mu.Unlock()
		view.push("玩家2 以新连接重新入场")

		if _, err := conn3.Write([]byte("WELCOME_BACK")); err == nil {
			view.push("已发送重连欢迎包")
		}
	}
}

func (v *serverView) push(msg string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.logs = append(v.logs, msg)
	if len(v.logs) > 5 {
		v.logs = v.logs[len(v.logs)-5:]
	}
}

func (v *serverView) renderLoop() {
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		v.mu.Lock()
		frame := v.buildFrameLocked()
		v.mu.Unlock()
		fmt.Print(sCls + frame)
	}
}

func (v *serverView) buildFrameLocked() string {
	var b strings.Builder

	b.WriteString(sCls)
	fmt.Fprintf(&b, "%s%s%s  TCP Reliable Lab  玩家终端  %s\n\n", sBold, sBgBlue, sWhite, sReset)
	fmt.Fprintf(&b, "  视角：%s服务器%s\n", sCyan, sReset)
	fmt.Fprintf(&b, "  阶段：%s%s%s\n\n", sYellow, v.phase, sReset)

	sBattlefield(&b, v)

	fmt.Fprintf(&b, "\n  玩家1生命 [%s]\n", sHP(v.player1.HP))
	fmt.Fprintf(&b, "  玩家2生命 [%s]\n", sHP(v.player2.HP))
	fmt.Fprintf(&b, "\n  现  象：%s%s%s\n", effectColor(v.ghost, v.conflict), effectText(v.ghost, v.conflict), sReset)
	fmt.Fprintf(&b, "\n  记录：\n")
	for _, log := range v.logs {
		fmt.Fprintf(&b, "  %s• %s%s\n", sDim, log, sReset)
	}

	return b.String()
}

func sBattlefield(b *strings.Builder, v *serverView) {
	grid := [5][9]string{}
	for y := range grid {
		for x := range grid[y] {
			grid[y][x] = "·"
		}
	}

	p1x, p1y := 1, 2
	p2x, p2y := 7, 2
	if v.player1.Online {
		grid[p1y][p1x] = sCyan + "1" + sReset
	}

	if v.player2.HP == 0 {
		grid[p2y][p2x] = sRed + "X" + sReset
	} else if v.player2.Online {
		grid[p2y][p2x] = sYellow + "2" + sReset
	}

	if v.player1.Online && v.player2.Online && v.player2.HP > 0 {
		grid[p2y][3] = sYellow + ">" + sReset
		grid[p2y][4] = sYellow + ">" + sReset
		grid[p2y][5] = sYellow + ">" + sReset
	}

	b.WriteString("\n  战场网格：\n")
	b.WriteString("  +-------------------+\n")
	for y := range grid {
		var row strings.Builder
		for x := range grid[y] {
			row.WriteString(grid[y][x])
			if x != len(grid[y])-1 {
				row.WriteString(" ")
			}
		}
		fmt.Fprintf(b, "  | %s |\n", row.String())
	}
	b.WriteString("  +-------------------+\n")
	fmt.Fprintf(b, "  %s1%s=玩家1  %s2%s=玩家2  %sX%s=服务器判死\n", sCyan, sReset, sYellow, sReset, sRed, sReset)
}

func sHP(hp int) string {
	total := 16
	fill := hp * total / 100
	if fill < 0 {
		fill = 0
	}
	if fill > total {
		fill = total
	}
	color := sGreen
	if hp <= 30 {
		color = sRed
	}
	return color + strings.Repeat("|", fill) + sReset + sDim + strings.Repeat(".", total-fill) + sReset + fmt.Sprintf(" %3d", hp)
}

func effectText(ghost, conflict bool) string {
	if conflict {
		return "服务器记忆中玩家2已死，但新连接客户端仍以满血出现"
	}
	if ghost {
		return "玩家2旧连接已断开，但写入时看起来仍然成功"
	}
	return "等待实验事件发生"
}

func effectColor(ghost, conflict bool) string {
	if conflict {
		return sRed
	}
	if ghost {
		return sYellow
	}
	return sGreen
}
