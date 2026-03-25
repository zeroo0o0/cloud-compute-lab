package main

import (
	"encoding/json"
	"fmt"
	"net"
)

// GameMessage 定义网络传输结构体
type GameMessage struct {
	Action    string  `json:"action"`
	PlayerID  int     `json:"player_id"`
	PositionX float64 `json:"pos_x"`
	PositionY float64 `json:"pos_y"`
}

func main() {
	fmt.Println("==================================================")
	fmt.Println("=== 实验 3 ：TCP 粘包灾难现场 ===")
	fmt.Println("==================================================")

	listener, err := net.Listen("tcp", "127.0.0.1:8888")
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
		
		// 【致命错误】：一次性把管子里的水全抽出来，根本不知道哪里是头哪里是尾
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
		
		// 试图反序列化
		err = json.Unmarshal(buffer[:n], &msg)
		if err != nil {
			fmt.Printf("[服务端崩溃] JSON 解析错误: %v\n", err)
			
		} else {
			fmt.Printf("[服务端] 解析成功, 收到第%d条消息\n",msgID)
		}

		msgID++
	}
}