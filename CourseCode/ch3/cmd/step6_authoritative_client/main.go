package main

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"warzone/exp6/internal/exp6proto"

	"golang.org/x/term"
)

func renderState(ws exp6proto.WorldState) {
	fmt.Printf("\r[frame=%d] ", ws.Frame)
	for _, p := range ws.Players {
		fmt.Printf("P%d(%d,%d,hp=%d) ", p.ID, p.X, p.Y, p.HP)
	}
	if ws.Event != "" {
		fmt.Printf("event=%s", ws.Event)
	}
	fmt.Print("\n按键控制: w/a/s/d移动, j攻击, q退出 > ")
}

func readSingleKey() (byte, error) {
	var buf [1]byte
	_, err := os.Stdin.Read(buf[:])
	return buf[0], err
}

func main() {
	host := "127.0.0.1"
	if len(os.Args) > 1 {
		host = os.Args[1]
	}
	conn, err := net.Dial("tcp", host+":9107")
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Println("=== Step6 权威客户端 ===")
	fmt.Println("连接到", host+":9107")
	fmt.Println("客户端只负责 发送输入 + 渲染服务器下发的权威状态")
	fmt.Println("输入: 直接按 w/a/s/d 移动, j攻击, q退出")
	if runtime.GOOS == "windows" {
		fmt.Println("提示: Windows 终端 raw 模式下，直接按键即可，无需回车")
	}
	fmt.Println()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// 收包 goroutine：持续接收服务器广播的权威状态
	go func() {
		lastFrame := -1
		var lastState string
		for {
			var ws exp6proto.WorldState
			if err := exp6proto.RecvJSON(conn, &ws); err != nil {
				fmt.Println("server disconnected:", err)
				os.Exit(0)
			}
			stateKey := fmt.Sprintf("%v|%s", ws.Players, ws.Event)
			if ws.Frame != lastFrame && stateKey != lastState {
				renderState(ws)
				lastFrame = ws.Frame
				lastState = stateKey
			}
		}
	}()

	// 主循环：单键输入 → 直接发给服务器
	fmt.Print("按键控制: w/a/s/d移动, j攻击, q退出 > ")
	for {
		action := "idle"
		b, err := readSingleKey()
		if err != nil {
			fmt.Println("read key err:", err)
			return
		}
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
		msg := exp6proto.InputMsg{Action: action}
		if err := exp6proto.SendJSON(conn, msg); err != nil {
			fmt.Println("send err:", err)
			return
		}
	}
}
