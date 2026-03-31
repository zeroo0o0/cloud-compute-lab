package main

import (
	"bufio"
	"ch3/internal/ch3proto"
	"ch3/internal/ch3render"
	"fmt"
	"net"
	"os"
	"strings"
)

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
	fmt.Println("=== zombie 单线程客户端(断网演示) ===")
	fmt.Println("连接到", host+":9107")

	reader := bufio.NewReader(os.Stdin)
	// 初始渲染：等待服务端在两位玩家就绪后下发首帧
	var initWS ch3proto.WorldState
	if err := ch3proto.RecvJSON(conn, &initWS); err != nil {
		fmt.Println("recv init err:", err)
		return
	}
	fmt.Print(ch3render.FormatWorldState(initWS, 20, 10))
	fmt.Print("\r\n")
	disconnected := false
	for {
		fmt.Print("输入: w/a/s/d 移动, j攻击, q退出, t模拟断网 (回车发送)|action> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("read input err:", err)
			return
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "t" {
			disconnected = !disconnected
			if disconnected {
				fmt.Println("simulate disconnect: ON (no send/recv)")
			} else {
				fmt.Println("simulate disconnect: OFF (resume)")
			}
			continue
		}
		if disconnected {
			fmt.Println("[disconnected] ignoring input, server will block waiting recv")
			continue
		}

		action := parseAction(line)
		if action == "" {
			action = "idle"
		}
		if action == "quit" {
			_ = ch3proto.SendJSON(conn, ch3proto.InputMsg{Action: "quit"})
			fmt.Println("quit")
			return
		}
		if err := ch3proto.SendJSON(conn, ch3proto.InputMsg{Action: action}); err != nil {
			fmt.Println("send err:", err)
			return
		}
		var ws ch3proto.WorldState
		if err := ch3proto.RecvJSON(conn, &ws); err != nil {
			fmt.Println("recv err:", err)
			return
		}
		fmt.Print(ch3render.FormatWorldState(ws, 20, 10))
		fmt.Print("\r\n")
	}
}

func parseAction(line string) string {
	line = strings.TrimSpace(strings.ToLower(line))
	switch line {
	case "w":
		return "up"
	case "s":
		return "down"
	case "a":
		return "left"
	case "d":
		return "right"
	case "j", "attack":
		return "attack"
	case "q", "quit":
		return "quit"
	case "", "idle":
		return "idle"
	default:
		return ""
	}
}
