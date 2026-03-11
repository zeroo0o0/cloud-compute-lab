package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
	"warzone/ch3/internal/ch3net"
	"warzone/ch3/internal/ch3proto"
	"warzone/ch3/internal/ch3render"

	"golang.org/x/term"
)

func readSingleKey() (byte, error) {
	var buf [1]byte
	_, err := os.Stdin.Read(buf[:])
	return buf[0], err
}

func parsePlayerID(args []string) int {
	if len(args) > 2 {
		if v, err := strconv.Atoi(args[2]); err == nil {
			return v
		}
	}
	return -1
}

func main() {
	host := "127.0.0.1"
	if len(os.Args) > 1 {
		host = os.Args[1]
	}
	playerID := parsePlayerID(os.Args)
	conn, err := net.Dial("tcp", host+":9108")
	if err != nil {
		panic(err)
	}
	rc := ch3net.NewReliableConn(conn)
	defer rc.Close()
	_ = rc.Send(ch3proto.JoinMsg{PlayerID: playerID})
	fmt.Println("=== Step7 ReliableConn 客户端 ===")
	fmt.Println("连接到", host+":9108")
	fmt.Println("特点: 使用 rc.Recv(50ms) 非阻塞收包，支持断线后按相同 playerID 重连")
	fmt.Println("输入: 直接按 w/a/s/d 移动, j 攻击, q 退出")
	fmt.Println("重连示例: go run ./cmd/exp7/reliable_client 127.0.0.1 0")
	if playerID < 0 {
		fmt.Println("警告: 未指定 playerID，服务器将拒绝连接。请使用 0 或 1。")
	}
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
			var ws ch3proto.WorldState
			err := rc.Recv(50*time.Millisecond, &ws)
			if err == nil {
				stateKey := fmt.Sprintf("%v|%s", ws.Players, ws.Event)
				if ws.Frame != lastFrame && stateKey != lastState {
					fmt.Printf("\r%s\n按键控制: w/a/s/d移动, j攻击, q退出 > ", ch3render.FormatWorldState(ws, 20, 10))
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
			msg := ch3proto.InputMsg{PlayerID: playerID, Action: action}
			if err := rc.Send(msg); err != nil {
				fmt.Println("send err:", err)
				return
			}
		}
	}
}
