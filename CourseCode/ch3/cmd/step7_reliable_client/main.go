package main

import (
	"fmt"
	"net"
	"os"
	"time"
	"warzone/exp6/internal/exp6net"
	"warzone/exp6/internal/exp6proto"

	"golang.org/x/term"
)

func renderState(ws exp6proto.WorldState) {
	fmt.Printf("\r[frame=%d] ", ws.Frame)
	for _, p := range ws.Players {
		fmt.Printf("P%d(%d,%d,hp=%d) ", p.ID, p.X, p.Y, p.HP)
	}
	if ws.Event != "" {
		fmt.Printf("| %s", ws.Event)
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
	conn, err := net.Dial("tcp", host+":9108")
	if err != nil {
		panic(err)
	}
	rc := exp6net.NewReliableConn(conn)
	defer rc.Close()
	fmt.Println("=== Step7 ReliableConn 客户端 ===")
	fmt.Println("连接到", host+":9108")
	fmt.Println("特点: 使用 rc.Recv(50ms) 非阻塞收包，超时则继续渲染循环")
	fmt.Println("输入: 直接按 w/a/s/d 移动, j 攻击, q 退出")
	fmt.Println()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	inputCh := make(chan byte, 8)
	go func() {
		for {
			b, err := readSingleKey()
			if err != nil {
				close(inputCh)
				return
			}
			inputCh <- b
		}
	}()

	// 主循环：非阻塞收包 + 非阻塞读输入 + 渲染
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	fmt.Print("按键控制: w/a/s/d移动, j攻击, q退出 > ")
	lastFrame := -1
	var lastState string

	for {
		select {
		case <-ticker.C:
			// 1. 非阻塞收取服务器状态 (50ms 超时)
			var ws exp6proto.WorldState
			err := rc.Recv(50*time.Millisecond, &ws)
			if err == nil {
				stateKey := fmt.Sprintf("%v|%s", ws.Players, ws.Event)
				if ws.Frame != lastFrame && stateKey != lastState {
					renderState(ws)
					lastFrame = ws.Frame
					lastState = stateKey
				}
			} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
				// 超时：丢帧但循环继续，不卡死
			} else {
				fmt.Println("server disconnected:", err)
				return
			}

		case b, ok := <-inputCh:
			if !ok {
				fmt.Println("input closed")
				return
			}
			// 2. 有键盘输入，发给服务器
			action := "idle"
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
			if err := rc.Send(msg); err != nil {
				fmt.Println("send err:", err)
				return
			}
		}
	}
}
