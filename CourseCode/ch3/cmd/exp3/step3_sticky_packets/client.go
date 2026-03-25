package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"bufio"
)

// GameMessage 定义网络传输结构体
type GameMessage struct {
	Action    string  `json:"action"`
	PlayerID  int     `json:"player_id"`
	PositionX float64 `json:"pos_x"`
	PositionY float64 `json:"pos_y"`
}

func main() {
	fmt.Println("=== 实验三：TCP 粘包灾难现场  ===")

	port := "8888"
	host := "127.0.0.1"
	if len(os.Args) > 1 {
		host = os.Args[1]
	}
	if len(os.Args) > 2 {
		port = os.Args[2]
	}
	addr := host + ":" + port

	runClient(addr)
}

// 灾难版发送：没有任何边界保护，直接序列化后死命往网卡里塞
func sendRawJSON(conn net.Conn, msg interface{}) {
	payload, _ := json.Marshal(msg)
	conn.Write(payload) // 【致命错误】去掉了 4 字节的长度前缀！
}

func runClient(addr string) {
	conn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("\n[客户端] 成功连接服务器。")
	
	// 创建标准输入读取器
	reader := bufio.NewReader(os.Stdin)
	round := 1

	for {
		// 等待用户按键
		messages := []GameMessage{
			{Action: "Move", PlayerID: 1001, PositionX: 10.5, PositionY: 20.0},
			{Action: "Attack", PlayerID: 1001, PositionX: 10.5, PositionY: 20.0},
			{Action: "Move", PlayerID: 1001, PositionX: 12.0, PositionY: 22.5},
		}

		for i, msg := range messages {
			fmt.Printf("\n[客户端] 第 %d 轮：按回车键继续发送消息（Ctrl+C 退出）...", round)
			reader.ReadString('\n')  // 阻塞等待用户输入任意内容后按回车

			fmt.Printf("[客户端] 第%d轮-消息%d: [玩家:%d | 动作:%-6s | 坐标:(%.1f, %.1f)]\n", 
				round, i+1, msg.PlayerID, msg.Action, msg.PositionX, msg.PositionY)
				
			sendRawJSON(conn, msg)
		}
		
		fmt.Printf("[客户端] 第 %d 轮发送完成（共3条消息）。\n", round)
		round++
	}
}