package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	p2Reset  = "\033[0m"
	p2Bold   = "\033[1m"
	p2Dim    = "\033[2m"
	p2Red    = "\033[91m"
	p2Green  = "\033[92m"
	p2Yellow = "\033[93m"
	p2Cyan   = "\033[96m"
	p2White  = "\033[97m"
	p2BgBlue = "\033[44m"
	p2Cls    = "\033[2J\033[H"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}

	hp1 := 100
	hp2 := 100
	renderPlayer2("首次上线", hp1, hp2, "在线", "未重连", "已进入战场", "")
	time.Sleep(600 * time.Millisecond)

	renderPlayer2("网络断开", hp1, hp2, "离线", "未重连", "旧连接已关闭", "")
	conn.Close()

	fmt.Print("\n>>> 按回车模拟玩家2重连 <<<")
	reader := bufio.NewReader(os.Stdin)
	reader.ReadBytes('\n')

	renderPlayer2("客户端重启", hp1, hp2, "离线", "准备重连", "正在建立新连接", "")
	time.Sleep(600 * time.Millisecond)

	reconnectConn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		fmt.Printf("[玩家2] 重连失败: %v\n", err)
		return
	}
	defer reconnectConn.Close()

	renderPlayer2("重新入场", hp1, hp2, "在线", "已重连", "本地仍显示100血", "")

	welcomeBuf := make([]byte, len("WELCOME_BACK"))
	if _, err := io.ReadFull(reconnectConn, welcomeBuf); err == nil {
		renderPlayer2("冲突出现", hp1, hp2, "在线", "已重连", "服务器接受新连接", strings.TrimSpace(string(welcomeBuf)))
	}
}

func renderPlayer2(phase string, hp1, hp2 int, netState, reconnectState, event, state string) {
	var b strings.Builder
	b.WriteString(p2Cls)
	fmt.Fprintf(&b, "%s%s%s  TCP Reliable Lab  玩家终端  %s\n\n", p2Bold, p2BgBlue, p2White, p2Reset)
	fmt.Fprintf(&b, "  视角：%s玩家2%s\n", p2Cyan, p2Reset)
	fmt.Fprintf(&b, "  阶段：%s%s%s\n\n", p2Yellow, phase, p2Reset)

	rows := []string{
		p2Line("角色", "玩家2", "定位", "观察方"),
		p2Line("网络", netState, "重连", reconnectState),
		p2Line("动作", "观察中", "目标", "玩家1"),
		p2Line("事件", event, "状态", "本地满血"),
	}
	p2Panel(&b, rows)

	fmt.Fprintf(&b, "\n  玩家1生命 [%s]\n", p2HP(hp1))
	fmt.Fprintf(&b, "  玩家2生命 [%s]\n", p2HP(hp2))
	if state != "" {
		fmt.Fprintf(&b, "  状态包：%s%s%s\n", p2Green, state, p2Reset)
	}
	fmt.Fprintf(&b, "  现  象：%s新连接建立后，本地角色仍按满血显示%s\n", p2Green, p2Reset)
	fmt.Print(b.String())
}

func p2Panel(b *strings.Builder, rows []string) {
	b.WriteString("  +----------------------------------------+\n")
	for _, row := range rows {
		fmt.Fprintf(b, "  | %s |\n", p2Pad(row, 38))
	}
	b.WriteString("  +----------------------------------------+\n")
}

func p2Line(k1, v1, k2, v2 string) string {
	return fmt.Sprintf("%s：%s  %s：%s", k1, v1, k2, v2)
}

func p2Pad(s string, width int) string {
	w := p2Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func p2Width(s string) int {
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

func p2HP(hp int) string {
	total := 16
	fill := hp * total / 100
	if fill < 0 {
		fill = 0
	}
	if fill > total {
		fill = total
	}
	color := p2Green
	if hp <= 30 {
		color = p2Red
	}
	return color + strings.Repeat("|", fill) + p2Reset + p2Dim + strings.Repeat(".", total-fill) + p2Reset + fmt.Sprintf(" %3d", hp)
}
