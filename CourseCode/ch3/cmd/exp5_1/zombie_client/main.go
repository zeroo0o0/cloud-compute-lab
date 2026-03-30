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
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

const (
	mapW = 20
	mapH = 10
)

// trimTrailingSpaces 去掉每一行末尾多余的空白字符，减少终端渲染抖动。
func trimTrailingSpaces(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n")
}

// sanitizeEvent 截断过长事件文本，避免压力场景下事件行污染终端显示。
func sanitizeEvent(event string) string {
	const maxRunes = 220
	r := []rune(event)
	if len(r) <= maxRunes {
		return event
	}
	return string(r[:maxRunes]) + " ...(truncated)"
}

// renderState 渲染一帧世界状态（统一使用普通换行输出）。
func renderState(ws ch3proto.WorldState) {
	ws.Event = sanitizeEvent(ws.Event)
	content := trimTrailingSpaces(ch3render.FormatWorldState(ws, mapW, mapH))
	fmt.Printf("\n%s\n输入: w/a/s/d 移动, j 攻击, q 退出, t 模拟断网 > ", content)
}

// renderFrozenHint 在 blackhole 模式下输出冻结提示，表明客户端仍在运行。
func renderFrozenHint(ws ch3proto.WorldState, since time.Duration) {
	// blackhole 时输出冻结提示，表明客户端仍在运行。
	fmt.Printf("\n[blackhole] frozen at frame=%d (%.1fs)", ws.Frame, since.Seconds())
}

// readSingleKey 从标准输入读取一个字节，供 raw 模式下单键控制使用。
func readSingleKey() (byte, error) {
	var buf [1]byte
	_, err := os.Stdin.Read(buf[:])
	return buf[0], err
}

// parsePlayerID 从命令行参数解析玩家 ID（0 或 1），解析失败返回 -1。
func parsePlayerID(args []string) int {
	// 用法：zombie_client [host] [playerID]
	if len(args) >= 3 {
		if v, err := strconv.Atoi(args[2]); err == nil {
			return v
		}
	}
	return -1
}

// main 启动 Step5.1 客户端：连接服务器、处理单键输入、并支持 blackhole 切换演示。
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
	if err := ch3proto.SendJSON(conn, ch3proto.JoinMsg{PlayerID: playerID}); err != nil {
		fmt.Println("join err:", err)
		return
	}

	fmt.Println("=== Step5.1 僵尸玩家演示客户端 ===")
	fmt.Println("连接到", host+":9110", "as player", playerID)
	fmt.Println("演示方法: 启动 2 个客户端后，让其中一个客户端进入 'blackhole' 模式模拟断网（不关闭连接）")
	fmt.Println("观察点: read-block 模式下僵尸连接会占住槽位，第 3 个客户端无法入场。")
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

	// 渲染门控：仅在“本客户端产生有效输入并成功发送”后，下一次状态更新才触发一次渲染。
	// 这样可以避免终端逐帧刷屏。
	var renderWanted atomic.Int32
	renderWanted.Store(1) // 首次连上时渲染一次，后续走输入驱动

	// 收包协程（可被 blackhole 开关暂停；正常模式下做去重）
	go func() {
		lastRenderedFrame := -1
		firstRender := true
		var frozenWs ch3proto.WorldState
		hasFrozen := false
		blackhole := false
		for {
			if blackhole {
				blackhole = <-blackholeCh
				// 可能切回 normal
				if !blackhole {
					fmt.Print("\n")
				}
				continue
			}

			// normal 模式：优先处理状态切换，其次阻塞收包
			select {
			case blackhole = <-blackholeCh:
				if blackhole {
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

			// 输入驱动渲染：只有本客户端产生有效输入时，才在下一次状态更新做一次渲染。
			if (firstRender || ws.Frame != lastRenderedFrame) && renderWanted.Swap(0) == 1 {
				renderState(ws)
				lastRenderedFrame = ws.Frame
				firstRender = false
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
		renderWanted.Store(1)
		// 小睡一下避免按键过快刷屏/占用过多带宽；演示不追求高帧率
		time.Sleep(10 * time.Millisecond)
	}
}
