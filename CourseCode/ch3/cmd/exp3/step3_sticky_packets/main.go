package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
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

	go runServer(listener)
	time.Sleep(500 * time.Millisecond)
	runClient()
	time.Sleep(2 * time.Second) // 等待所有日志打印完毕
}

// 灾难版发送：没有任何边界保护，直接序列化后死命往网卡里塞
func sendRawJSON(conn net.Conn, msg interface{}) {
	payload, _ := json.Marshal(msg)
	conn.Write(payload) // 【致命错误】去掉了 4 字节的长度前缀！
}

func runServer(listener net.Listener) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	fmt.Println("\n[服务端] 成功接收客户端连接！")
	fmt.Println("[服务端] 正在刻意休眠 1 秒钟，暂时不读取网卡数据...")
	

	// 【核心时序控制】：强行休眠，让网卡里的无边界字节流全部挤在一起
	time.Sleep(1 * time.Second)

	
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
		fmt.Println("[服务端] 解析成功 (这在粘包情况下是不可能的)")
	}
}

func runClient() {
	conn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("\n[客户端] 成功连接服务器！准备瞬间连续发送 3 条消息 (不加长度头)...")

	messages := []GameMessage{
		{Action: "Move", PlayerID: 1001, PositionX: 10.5, PositionY: 20.0},
		{Action: "Attack", PlayerID: 1001, PositionX: 10.5, PositionY: 20.0},
		{Action: "Move", PlayerID: 1001, PositionX: 12.0, PositionY: 22.5},
	}

	for i, msg := range messages {
		// 【对齐正确版】：在发送前，将即将被序列化的结构体内容清晰地打印出来
		fmt.Printf("[客户端] 正在将第 %d 条消息流推入网卡 -> 原始内容: [玩家:%d | 动作:%-6s | 坐标:(%.1f, %.1f)]\n", 
			i+1, msg.PlayerID, msg.Action, msg.PositionX, msg.PositionY)
		
		// 丢弃了包头的发送方式
		sendRawJSON(conn, msg)
	}
	
	fmt.Println("[客户端] 3 条消息已强制推入底层 TCP 协议栈！")
}