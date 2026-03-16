package main

import (
	"ch3/internal/ch3proto"
	"ch3/internal/ch3render"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

const (
	mapW = 20
	mapH = 10
)

// 是否使用 ANSI 光标控制做“原地刷新”。
// Windows Terminal / VS Code 终端通常支持；老的 conhost 可能不完整。
func ansiOK() bool {
	if os.Getenv("NO_ANSI") != "" {
		return false
	}
	// Windows 下简化判断：Windows Terminal/ConPTY 环境一般会设置这些变量
	if runtime.GOOS == "windows" {
		if os.Getenv("WT_SESSION") != "" || os.Getenv("TERM") != "" || os.Getenv("ConEmuANSI") == "ON" {
			return true
		}
		return false
	}
	return os.Getenv("TERM") != ""
}

func clearScreenANSI() {
	// 清屏 + 光标回到左上
	fmt.Print("\x1b[2J\x1b[H")
}

func trimTrailingSpaces(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n")
}

func renderState(ws ch3proto.WorldState) {
	content := trimTrailingSpaces(ch3render.FormatWorldState(ws, mapW, mapH))
	if ansiOK() {
		// 原地刷新：避免每一帧都往下滚动造成“刷屏”。
		clearScreenANSI()
		fmt.Printf("%s\n输入: w/a/s/d 移动, j 攻击, q 退出, t 模拟断网 > ", content)
		return
	}
	// 兼容回退：不支持 ANSI 就只能换行输出（会滚屏）
	fmt.Printf("\n%s\n输入: w/a/s/d 移动, j 攻击, q 退出, t 模拟断网 > ", content)
}

func renderFrozenHint(ws ch3proto.WorldState, since time.Duration) {
	// blackhole 时避免重复刷整屏（课堂上容易被认为“刷屏”）。
	// 用单行心跳提示“客户端还活着”，同时标出冻结在第几帧。
	if ansiOK() {
		// 单行更新，但不依赖清屏；控制台缩放时 \r 可能造成错位，这是终端行为限制。
		// 如果你觉得缩放时还是会乱，可以设置环境变量 NO_ANSI=1 强制走回退方案。
		fmt.Printf("\r[blackhole] frozen at frame=%d (%.1fs)  ", ws.Frame, since.Seconds())
		return
	}
	// 回退方案：直接换行输出（不会覆盖错位，但会滚屏；频率已经很低）
	fmt.Printf("\n[blackhole] frozen at frame=%d (%.1fs)", ws.Frame, since.Seconds())
}

func readSingleKey() (byte, error) {
	var buf [1]byte
	_, err := os.Stdin.Read(buf[:])
	return buf[0], err
}

func parsePlayerID(args []string) int {
	// 用法：zombie_client [host] [playerID]
	if len(args) >= 3 {
		if v, err := strconv.Atoi(args[2]); err == nil {
			return v
		}
	}
	return -1
}

func main() {
	host := "127.0.0.1"
	if len(os.Args) >= 2 {
		host = os.Args[1]
	}
	playerID := parsePlayerID(os.Args)
	if playerID != 0 && playerID != 1 {
		fmt.Println("用法: go run ./cmd/exp5_1/zombie_client 127.0.0.1 0  # 或 1")
		return
	}

	conn, err := net.Dial("tcp", host+":9110")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("=== Step5.1 僵尸玩家演示客户端 ===")
	fmt.Println("连接到", host+":9110", "as player", playerID)
	fmt.Println("演示方法: 启动 2 个客户端后，让其中一个客户端进入 'blackhole' 模式模拟断网（不关闭连接）")
	fmt.Println("热键: 按 t 切换 blackhole(模拟断网) / normal(正常联网)")
	if runtime.GOOS == "windows" {
		fmt.Println("Windows 说明: 因为 client/server 同机运行，不方便拔网线；推荐直接用 t 热键模拟半开/僵尸玩家")
	}
	fmt.Println()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// blackhole=true 表示“模拟断网/半开”：保持 TCP 连接不关闭，但不收包也不发包。
	// UI 行为：冻结在断网前最后一帧；blackhole 期间只输出单行心跳提示，避免刷屏。
	blackholeCh := make(chan bool, 1)
	blackholeCh <- false

	freezeRenderTick := time.NewTicker(700 * time.Millisecond)
	defer freezeRenderTick.Stop()

	// 收包协程（可被 blackhole 开关暂停；正常模式下做去重）
	go func() {
		lastFrame := -1
		var frozenWs ch3proto.WorldState
		hasFrozen := false
		blackhole := false
		freezeSince := time.Time{}
		for {
			if blackhole {
				select {
				case blackhole = <-blackholeCh:
					// 可能切回 normal
					if !blackhole {
						fmt.Print("\n")
					}
				case <-freezeRenderTick.C:
					if hasFrozen {
						renderFrozenHint(frozenWs, time.Since(freezeSince))
					}
				}
				continue
			}

			// normal 模式：优先处理状态切换，其次阻塞收包
			select {
			case blackhole = <-blackholeCh:
				if blackhole {
					// 进入冻结：冻结在“进入 blackhole 前最后一次收到的 ws”。
					freezeSince = time.Now()
					if hasFrozen {
						renderState(frozenWs)
						renderFrozenHint(frozenWs, 0)
					}
				}
			default:
			}

			var ws ch3proto.WorldState
			if err := ch3proto.RecvJSON(conn, &ws); err != nil {
				fmt.Println("\nserver disconnected:", err)
				os.Exit(0)
			}
			// 记录最新状态，用于 blackhole 冻结（只在 normal 模式更新）。
			frozenWs = ws
			hasFrozen = true

			// 去刷屏：只要帧号变化才渲染一次。
			// 注意：服务器的 spam 消息不是 WorldState，解码会失败并断开；
			// 所以 server 端 spam 必须保持为“可被忽略”或 client 只读 WorldState。
			if ws.Frame != lastFrame {
				renderState(ws)
				lastFrame = ws.Frame
			}
		}
	}()

	// 主循环：单键输入 -> 发送给服务器（不带帧号，服务器按“上一帧以来收到的最后一次输入”处理）
	blackhole := false
	fmt.Print("输入: w/a/s/d 移动, j 攻击, q 退出, t 模拟断网 > ")
	for {
		b, err := readSingleKey()
		if err != nil {
			fmt.Println("read key err:", err)
			return
		}

		// 回显输入的字符（raw 模式默认不会自动回显）
		// 让课堂演示时能直观看到 keypress 确实被读入。
		fmt.Printf("%c", b)

		if b == 't' || b == 'T' {
			blackhole = !blackhole
			// 通知收包协程切换状态
			select {
			case blackholeCh <- blackhole:
			default:
				// 丢弃旧状态，确保最新状态可达
				<-blackholeCh
				blackholeCh <- blackhole
			}
			mode := "normal"
			if blackhole {
				mode = "blackhole (simulate offline / half-open)"
			}
			fmt.Println("\n[client] network mode =>", mode)
			continue
		}
		action := ""
		switch b {
		case 'w', 'W':
			action = "up"
		case 's', 'S':
			action = "down"
		case 'a', 'A':
			action = "left"
		case 'd', 'D':
			action = "right"
		case 'j', 'J':
			action = "attack"
		case 'q', 'Q':
			fmt.Println("quit")
			return
		default:
			continue
		}
		if blackhole {
			// 模拟断网：不发送输入，但进程和 socket 都保持着
			fmt.Println("\n[client] dropped input (blackhole):", action)
			continue
		}
		msg := ch3proto.InputMsg{PlayerID: playerID, Action: action}
		if err := ch3proto.SendJSON(conn, msg); err != nil {
			fmt.Println("send err:", err)
			return
		}
		// 小睡一下避免按键过快刷屏/占用过多带宽；演示不追求高帧率
		time.Sleep(10 * time.Millisecond)
	}
}
