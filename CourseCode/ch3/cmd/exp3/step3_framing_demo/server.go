package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	// "time"
)

// GameMessage 定义网络传输结构体
type GameMessage struct {
	Action    string  `json:"action"`
	PlayerID  int     `json:"player_id"`
	PositionX float64 `json:"pos_x"`
	PositionY float64 `json:"pos_y"`
}

func main() {
	fmt.Println("=== 实验三：TCP 粘包处理与 JSON 序列化演示  ===")

	listener, err := net.Listen("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	runServer(listener)
}

// 核心函数 2：接收并拆包 (按长度头读取)
func recvJSON(conn net.Conn, msg interface{}) error {
	var length uint32
	err := binary.Read(conn, binary.BigEndian, &length)
	if err != nil {
		return err
	}
	payload := make([]byte, length)
	// io.ReadFull 保证精确读取指定的字节数
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
	// fmt.Println("[服务端] 休眠 1 秒钟，暂时不读取网卡数据...")
	// fmt.Println("[服务端] (等待客户端的多条消息在 TCP 底层缓冲区中发生物理堆积)")

	// time.Sleep(1 * time.Second) 

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