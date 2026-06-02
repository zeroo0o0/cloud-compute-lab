package main

import (
	"encoding/json"
	"fmt"
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

	fmt.Println("==================================================")
	fmt.Println("=== 实验三：TCP 粘包灾难现场（服务端） ===")
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

	fmt.Println("\n[服务端] 成功接收客户端连接！")

	msgID := 1
	for {
		buffer := make([]byte, 1024)

		/*
			================ 【学生重点 第三章：粘包错误版】 ================
			这里故意直接 Read 裸 TCP 字节流。
			一次 Read 可能读到半条 JSON，也可能读到多条 JSON 连在一起。
			JSON 解析器看到的是“没有边界的连续字节”，所以会随机失败。
			这个错误版用来和 step3_framing_demo 的长度前缀方案对照。
			============================================================
		*/
		n, err := conn.Read(buffer)
		if err != nil {
			return
		}

		rawString := string(buffer[:n])
		fmt.Println("\n>>> 【服务端】一次性读到了以下连体字节流：")
		fmt.Println(rawString)
		fmt.Println("<<<")

		var msg GameMessage
		fmt.Println("\n[服务端] 将粘在一起的数据交给 JSON 解析器...")

		err = json.Unmarshal(buffer[:n], &msg)
		if err != nil {
			fmt.Printf("[服务端崩溃] JSON 解析错误: %v\n", err)
		} else {
			fmt.Printf("[服务端] 解析成功, 收到第%d条消息\n", msgID)
		}

		msgID++
	}
}
