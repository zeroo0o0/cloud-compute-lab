package main

import (
	"bufio"
	"encoding/binary"
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

	runClient(addr)
}

func sendJSON(conn net.Conn, msg any) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	length := uint32(len(payload))
	err = binary.Write(conn, binary.BigEndian, length)
	if err != nil {
		return err
	}
	_, err = conn.Write(payload)
	return err
}

func runClient(addr string) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("\n[客户端] 成功连接服务器。")

	reader := bufio.NewReader(os.Stdin)
	round := 1

	for {
		fmt.Printf("\n[客户端] 第 %d 轮：按回车键继续发送消息（Ctrl+C 退出）...", round)
		if _, err := reader.ReadString('\n'); err != nil {
			fmt.Println("\n[客户端] 输入结束，退出。")
			return
		}

		fmt.Printf("[客户端] 开始发送第 %d 轮消息...\n", round)

		messages := []GameMessage{
			{Action: "Move", PlayerID: 1001, PositionX: 10.5, PositionY: 20.0},
			{Action: "Attack", PlayerID: 1001, PositionX: 10.5, PositionY: 20.0},
			{Action: "Move", PlayerID: 1001, PositionX: 12.0, PositionY: 22.5},
		}

		for i, msg := range messages {
			fmt.Printf("[客户端] 第%d轮-消息%d: [玩家:%d | 动作:%-6s | 坐标:(%.1f, %.1f)]\n",
				round, i+1, msg.PlayerID, msg.Action, msg.PositionX, msg.PositionY)

			err := sendJSON(conn, msg)
			if err != nil {
				fmt.Printf("[客户端] 第%d轮发送失败: %v\n", round, err)
				return
			}
		}

		fmt.Printf("[客户端] 第 %d 轮发送完成（共3条消息）。\n", round)
		round++
	}
}
