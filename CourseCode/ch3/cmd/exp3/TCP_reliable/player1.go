package main

import (
	"fmt"
	"io"
	"net"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	p1Reset  = "\033[0m"
	p1Bold   = "\033[1m"
	p1Dim    = "\033[2m"
	p1Red    = "\033[91m"
	p1Green  = "\033[92m"
	p1Yellow = "\033[93m"
	p1Cyan   = "\033[96m"
	p1White  = "\033[97m"
	p1BgBlue = "\033[44m"
	p1Cls    = "\033[2J\033[H"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	renderPlayer1("等待目标", 100, 100, "等待玩家2上线", "未开火", "")

	signalBuf := make([]byte, len("P2_ONLINE"))
	if _, err := io.ReadFull(conn, signalBuf); err != nil {
		fmt.Println("读取上线信号失败:", err)
		return
	}

	renderPlayer1("锁定完成", 100, 100, "已发现玩家2", "瞄准中", "")
	time.Sleep(700 * time.Millisecond)

	if string(signalBuf) == "P2_ONLINE" {
		if _, err := conn.Write([]byte("ATTACK")); err != nil {
			fmt.Println("发送攻击失败:", err)
			return
		}
		renderPlayer1("发起攻击", 100, 0, "攻击指令已发送", "已开火", "")
	}

	msg := make([]byte, len("STATE: Player2 DEAD    "))
	if _, err := io.ReadFull(conn, msg); err != nil {
		fmt.Println("读取结算消息失败:", err)
		return
	}

	renderPlayer1("攻击生效", 100, 0, "服务器确认玩家2死亡", "已命中", strings.TrimSpace(string(msg)))
}

func renderPlayer1(phase string, hp1, hp2 int, event, action, state string) {
	var b strings.Builder
	b.WriteString(p1Cls)
	fmt.Fprintf(&b, "%s%s%s  TCP Reliable Lab  玩家终端  %s\n\n", p1Bold, p1BgBlue, p1White, p1Reset)
	fmt.Fprintf(&b, "  视角：%s玩家1%s\n", p1Cyan, p1Reset)
	fmt.Fprintf(&b, "  阶段：%s%s%s\n\n", p1Yellow, phase, p1Reset)

	rows := []string{
		p1Line("角色", "玩家1", "定位", "攻击方"),
		p1Line("网络", "在线", "重连", "无"),
		p1Line("动作", action, "目标", targetState(hp2)),
		p1Line("事件", event, "状态", "旧连接"),
	}
	p1Panel(&b, rows)

	fmt.Fprintf(&b, "\n  玩家1生命 [%s]\n", p1HP(hp1))
	fmt.Fprintf(&b, "  玩家2生命 [%s]\n", p1HP(hp2))
	if state != "" {
		fmt.Fprintf(&b, "\n  状态包：%s%s%s\n", p1Green, state, p1Reset)
	}
	fmt.Fprintf(&b, "  现  象：%s攻击与死亡消息在这条连接中可靠送达%s\n", p1Green, p1Reset)
	fmt.Print(b.String())
}

func p1Panel(b *strings.Builder, rows []string) {
	b.WriteString("  +----------------------------------------+\n")
	for _, row := range rows {
		fmt.Fprintf(b, "  | %s |\n", p1Pad(row, 38))
	}
	b.WriteString("  +----------------------------------------+\n")
}

func p1Line(k1, v1, k2, v2 string) string {
	return fmt.Sprintf("%s：%s  %s：%s", k1, v1, k2, v2)
}

func p1Pad(s string, width int) string {
	w := p1Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func p1Width(s string) int {
	width := 0
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		if r <= 127 {
			width++
		} else {
			width += 2
		}
	}
	return width
}

func p1HP(hp int) string {
	total := 16
	fill := hp * total / 100
	if fill < 0 {
		fill = 0
	}
	if fill > total {
		fill = total
	}
	color := p1Green
	if hp <= 30 {
		color = p1Red
	}
	return color + strings.Repeat("|", fill) + p1Reset + p1Dim + strings.Repeat(".", total-fill) + p1Reset + fmt.Sprintf(" %3d", hp)
}

func targetState(hp int) string {
	if hp <= 0 {
		return "已击倒"
	}
	return "存活"
}
