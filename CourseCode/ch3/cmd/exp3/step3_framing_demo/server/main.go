package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
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

	fmt.Println("=== 实验三：TCP 粘包处理与 JSON 序列化演示（服务端） ===")
	fmt.Println("[服务端] listen", addr)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	runServer(listener)
}

func recvJSON(conn net.Conn, msg any) error {
	var length uint32
	err := binary.Read(conn, binary.BigEndian, &length)
	if err != nil {
		return err
	}
	/*
		================ 【学生重点 第三章：粘包修复版】 ================
		长度前缀把连续 TCP 字节流切回一条条应用层消息：
		1. 先读 4 字节，得到下一条 JSON 的长度。
		2. 再用 io.ReadFull 精确读满这条 JSON。
		3. 最后才交给 json.Unmarshal。
		============================================================
	*/
	payload := make([]byte, length)
	_, err = io.ReadFull(conn, payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, msg)
}

func runServer(listener net.Listener) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	fmt.Println("\n[服务端] 成功接收客户端连接！")
	fmt.Println("\n[服务端] 按照【4字节长度前缀】切割字节流...")

	msgID := 1
	for {
		var msg GameMessage
		err := recvJSON(conn, &msg)
		if err != nil {
			if err == io.EOF {
				fmt.Println("[服务端] 客户端已断开连接，所有消息处理完毕。")
			} else {
				fmt.Println("[服务端] 解析数据出错:", err)
			}
			return
		}

		fmt.Printf("[服务端-成功解包] 收到第%d条指令 -> [玩家:%d | 动作:%-6s | 坐标:(%.1f, %.1f)]\n",
			msgID, msg.PlayerID, msg.Action, msg.PositionX, msg.PositionY)

		msgID++
	}
}
