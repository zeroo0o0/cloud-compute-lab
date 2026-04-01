// client 是 BattleWorld Lab1 的异步多线程客户端程序。
//
// 启动方式：
//   go run ./cmd/client
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"battleworld/protocol"
)

const serverAddr = "localhost:9000"

// ─── 客户端全局 UI 状态 ────────────────────────────────────────────────────
var (
	latestSnapshot protocol.Message
	myID           int
	eventLog       []string
	uiMu           sync.Mutex
)

func addEvent(text string) {
	uiMu.Lock()
	defer uiMu.Unlock()
	eventLog = append(eventLog, text)
	if len(eventLog) > 6 {
		eventLog = eventLog[len(eventLog)-6:]
	}
}

func updateSnapshot(msg protocol.Message) {
	uiMu.Lock()
	defer uiMu.Unlock()
	latestSnapshot = msg
}

func drawUI() {
	uiMu.Lock()
	defer uiMu.Unlock()

	var sb strings.Builder
	sb.WriteString("\033[H")
	sb.WriteString("═══ BattleWorld 战场 (Lab1) ═══\033[K\n")

	grid := make([][]string, protocol.MapHeight)
	for i := range grid {
		grid[i] = make([]string, protocol.MapWidth)
		for j := range grid[i] {
			grid[i][j] = " . "
		}
	}

	for _, p := range latestSnapshot.Players {
		if p.X >= 0 && p.X < protocol.MapWidth && p.Y >= 0 && p.Y < protocol.MapHeight {
			if !p.Alive {
				grid[p.Y][p.X] = " x "
			} else if p.ID == myID {
				grid[p.Y][p.X] = " @ "
			} else {
				grid[p.Y][p.X] = " * "
			}
		}
	}

	for _, row := range grid {
		for _, cell := range row {
			sb.WriteString(cell)
		}
		sb.WriteString("\033[K\n")
	}

	sb.WriteString("\n─── 战场快报 ─────────────────────────────────────────\033[K\n")
	for _, p := range latestSnapshot.Players {
		tag := "  "
		if p.ID == myID {
			tag = "▶ "
		}
		status := "存活"
		if !p.Alive {
			status = "☠ 阵亡"
		}
		bar := hpBar(p.HP, p.MaxHP, 10)
		sb.WriteString(fmt.Sprintf("%s%-10s %s 💊%d 📍(%2d,%2d) %s\033[K\n",
			tag, p.Name, bar, p.Potions, p.X, p.Y, status))
	}
	sb.WriteString("──────────────────────────────────────────────────────\033[K\n")

	for _, e := range eventLog {
		sb.WriteString("📢 " + e + "\033[K\n")
	}

	sb.WriteString("\033[J")
	sb.WriteString("> \033[K")

	fmt.Print(sb.String())
}

func hpBar(hp, maxHP, w int) string {
	filled := 0
	if maxHP > 0 {
		filled = hp * w / maxHP
	}
	return fmt.Sprintf("HP[%s%s]%3d",
		strings.Repeat("█", filled), strings.Repeat("░", w-filled), hp)
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("请输入你的名字: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		name = "无名勇士"
	}

	fmt.Print("\033[?1049h\033[2J\033[H")
	defer fmt.Print("\033[?1049l")

	// ╔═════════════════════════════════════════════════════════════════════════╗
	// ║  任务 C-3：连接服务器                                                  ║
	// ║                                                                         ║
	// ║  功能：通过 TCP 连接到服务器地址 serverAddr。                            ║
	// ║                                                                         ║
	// ║  实现要点：                                                             ║
	// ║    调用 net.Dial("tcp", serverAddr)，将返回值存入 raw 和 err。           ║
	// ║    若 err != nil，向 os.Stderr 打印错误信息后 os.Exit(1)。              ║
	// ║    使用 defer raw.Close() 确保程序退出时关闭连接。                       ║
	// ║                                                                         ║
	// ║  提示框架：                                                             ║
	// ║    raw, err := net.Dial(???, ???)                                        ║
	// ║    if err != nil { ... }                                                ║
	// ║    defer raw.Close()                                                    ║
	// ╚═════════════════════════════════════════════════════════════════════════╝

	// TODO C-3: 在此处连接服务器
	panic("C-3 尚未实现：请完成 net.Dial 调用")

	conn := protocol.NewConn(raw)
	conn.Send(protocol.Message{Type: protocol.TypeJoin, Text: name})

	addEvent("✅ 已连接服务器，等待游戏开始...")
	drawUI()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			msg, err := conn.Receive()
			if err != nil {
				addEvent("与服务器连接断开。")
				drawUI()
				return
			}
			switch msg.Type {
			case protocol.TypeInit:
				myID = msg.YourID
				addEvent(fmt.Sprintf("🎮 %s", msg.Text))
				drawUI()
			case protocol.TypeState:
				updateSnapshot(msg)
				drawUI()
			case protocol.TypeEvent:
				addEvent(msg.Text)
				drawUI()
			case protocol.TypeYourTurn:
				addEvent(fmt.Sprintf("⚡ %s", msg.Text))
				drawUI()
			case protocol.TypeWait:
				addEvent(fmt.Sprintf("⏳ %s", msg.Text))
				drawUI()
			case protocol.TypeGameOver:
				addEvent(fmt.Sprintf("🏁 游戏结束！获胜者：【%s】", msg.Winner))
				drawUI()
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		default:
		}

		in, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		in = strings.TrimSpace(strings.ToLower(in))
		if in == "" {
			drawUI()
			continue
		}

		var msg protocol.Message
		switch in {
		case "w":
			msg = protocol.Message{Type: protocol.TypeMove, Dir: protocol.DirUp}
		case "s":
			msg = protocol.Message{Type: protocol.TypeMove, Dir: protocol.DirDown}
		case "a":
			msg = protocol.Message{Type: protocol.TypeMove, Dir: protocol.DirLeft}
		case "d":
			msg = protocol.Message{Type: protocol.TypeMove, Dir: protocol.DirRight}
		case "f":
			msg = protocol.Message{Type: protocol.TypeAttack}
		case "h":
			msg = protocol.Message{Type: protocol.TypeHeal}
		case "?", "help":
			addEvent("操作指令: w/s/a/d(移动) f(攻击) h(药水) q(退出)")
			drawUI()
			continue
		case "q", "quit":
			return
		default:
			addEvent("⚠ 未知指令，输入 ? 查看帮助")
			drawUI()
			continue
		}
		conn.Send(msg)
		drawUI()
	}
}