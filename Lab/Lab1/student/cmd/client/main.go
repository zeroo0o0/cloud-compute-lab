// client 是 BattleWorld Lab1 的异步多线程客户端程序。
//
// 启动方式：
//
//	go run ./cmd/client
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"battleworld/protocol"
)

const (
	serverAddr    = "localhost:9000"
	minTermWidth  = 70
	minTermHeight = 32
)

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
	if text == "" {
		return
	}
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

func setMyID(id int) {
	uiMu.Lock()
	defer uiMu.Unlock()
	myID = id
}

func drawUI() {
	uiMu.Lock()
	defer uiMu.Unlock()

	var sb strings.Builder
	sb.WriteString("\033[H")

	cols, rows, ok := getTerminalSize()
	if ok && (cols < minTermWidth || rows < minTermHeight) {
		sb.WriteString("═══ BattleWorld 战场 (Lab1) ═══\033[K\n")
		sb.WriteString(fmt.Sprintf("⚠ 终端窗口过小：当前 %d×%d，至少需要 %d×%d\033[K\n", cols, rows, minTermWidth, minTermHeight))
		sb.WriteString("请放大终端窗口后继续操作。\033[K\n")
		sb.WriteString("\033[J")
		fmt.Print(sb.String())
		return
	}

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

func enterAltScreen() {
	fmt.Print("\033[?1049h\033[2J\033[H")
}

func leaveAltScreen() {
	fmt.Print("\033[?1049l")
}

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

func getTerminalSize() (int, int, bool) {
	ws := &winsize{}
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		os.Stdout.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)),
	)
	if errno != 0 || ws.Col == 0 || ws.Row == 0 {
		return 0, 0, false
	}
	return int(ws.Col), int(ws.Row), true
}

// waitForUsableTerminal 在程序最开始就检查终端尺寸。
// 若窗口太小，则不允许进入游戏，只显示“终端太小”的提示。
func waitForUsableTerminal() {
	for {
		cols, rows, ok := getTerminalSize()
		if !ok || (cols >= minTermWidth && rows >= minTermHeight) {
			fmt.Print("\033[2J\033[H")
			return
		}

		fmt.Print("\033[2J\033[H")
		fmt.Printf("═══ BattleWorld 战场 (Lab1) ═══\n")
		fmt.Printf("⚠ 终端窗口过小：当前 %d×%d，至少需要 %d×%d\n", cols, rows, minTermWidth, minTermHeight)
		fmt.Printf("请先放大终端窗口，再进入游戏。\n")
		time.Sleep(300 * time.Millisecond)
	}
}

func promptName(reader *bufio.Reader, prompt string) string {
	for {
		fmt.Print(prompt)
		name, err := reader.ReadString('\n')
		if err != nil {
			return "无名勇士"
		}
		name = strings.TrimSpace(name)
		if name == "" {
			fmt.Println("名字不能为空，请重新输入。")
			continue
		}
		return name
	}
}

// joinUntilAccepted 在进入游戏前完成名字协商。
// 若服务器提示重名，则客户端原地重新输入名字并再次发送 join。
func joinUntilAccepted(conn *protocol.Conn, reader *bufio.Reader, firstName string) (protocol.Message, error) {
	name := firstName
	for {
		if err := conn.Send(protocol.Message{Type: protocol.TypeJoin, Text: name}); err != nil {
			return protocol.Message{}, err
		}

		for {
			msg, err := conn.Receive()
			if err != nil {
				return protocol.Message{}, err
			}

			switch msg.Type {
			case protocol.TypeInit:
				return msg, nil
			case protocol.TypeEvent:
				fmt.Println(msg.Text)
				if strings.Contains(msg.Text, "名字已被使用") || strings.Contains(msg.Text, "重新输入名字") {
					name = promptName(reader, "请重新输入名字: ")
					goto retryJoin
				}
			default:
				// 登录阶段忽略其他类型消息。
			}
		}
	retryJoin:
	}
}

func main() {
	// 按你的要求：终端检查放在最开始。
	waitForUsableTerminal()

	reader := bufio.NewReader(os.Stdin)
	name := promptName(reader, "请输入你的名字: ")

	raw, err := net.Dial("tcp", serverAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接服务器失败: %v\n", err)
		return
	}
	defer raw.Close()

	conn := protocol.NewConn(raw)
	initMsg, err := joinUntilAccepted(conn, reader, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "登录失败: %v\n", err)
		return
	}

	enterAltScreen()
	defer leaveAltScreen()

	setMyID(initMsg.YourID)
	addEvent(fmt.Sprintf("🎮 %s", initMsg.Text))
	drawUI()

	done := make(chan struct{})
	inputCh := make(chan string)

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

	go func() {
		defer close(inputCh)
		for {
			in, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			inputCh <- strings.TrimSpace(strings.ToLower(in))
		}
	}()

	for {
		select {
		case <-done:
			return
		case in, ok := <-inputCh:
			if !ok {
				return
			}

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

			if err := conn.Send(msg); err != nil {
				addEvent("⚠ 指令发送失败，连接可能已断开")
				drawUI()
				return
			}
			drawUI()
		}
	}
}
