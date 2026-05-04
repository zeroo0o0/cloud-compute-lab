package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"ch3/internal/ch3proto"
	"ch3/internal/ch3render"
)

type GameMessage struct {
	Action    string  `json:"action"`
	PlayerID  int     `json:"player_id"`
	PositionX float64 `json:"pos_x"`
	PositionY float64 `json:"pos_y"`
}

func main() {
	addr := "127.0.0.1:8888"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	fmt.Println("==================================================")
	fmt.Println("====== 实验三：游戏化 TCP 粘包服务端 ======")
	fmt.Println("==================================================")
	fmt.Println("[服务端] listen", addr)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	runServer(listener)
}

func runServer(listener net.Listener) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	ws := ch3proto.WorldState{
		Frame: 0,
		Players: []ch3proto.PlayerState{
			{ID: 1, X: 5, Y: 5, HP: 100, Online: true},
		},
		Event: "服务器就绪，等待玩家输入...",
	}

	fmt.Println("\n>>> [服务端] --- 游戏初始状态")
	fmt.Println(ch3render.FormatWorldState(ws, 28, 10))

	buffer := make([]byte, 1024)

	n, err := conn.Read(buffer)
	if err == nil {
		var msg GameMessage
		err = json.Unmarshal(buffer[:n], &msg)
		ws.Frame++
		if err == nil {
			ws.Players[0].X = int(msg.PositionX)
			ws.Players[0].Y = int(msg.PositionY)
			ws.Event = "【Nagle放行】第1个包到达，解析成功，玩家移动！"

			time.Sleep(200 * time.Millisecond)

			fmt.Println("\n>>> [服务端] --- 正常接收第 1 步 (Y=6)")
			fmt.Println(ch3render.FormatWorldState(ws, 28, 10))
		}
	}

	time.Sleep(500 * time.Millisecond)

	n, err = conn.Read(buffer)
	if err == nil {
		var msg GameMessage
		err = json.Unmarshal(buffer[:n], &msg)
		ws.Frame++
		if err != nil {
			ws.Event = fmt.Sprintf("【Nagle拼车灾难】收到连体包 (%d 字节)，解析崩溃!", n)

			fmt.Println("\n>>> [服务端] --- 发生 TCP 粘包 (卡死在 Y=6)")
			fmt.Println(ch3render.FormatWorldState(ws, 28, 10))

			fmt.Println("\n[底层缓冲区残留的连体数据]：")
			fmt.Println(string(buffer[:n]))
		}
	}
}
