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

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(false)
	}

	fmt.Println("\n[客户端] 玩家快速连续按下了三次【向下移动】键,期望走到 Y=8 ...")

	messages := []GameMessage{
		{Action: "Move", PlayerID: 1, PositionX: 5.0, PositionY: 6.0},
		{Action: "Move", PlayerID: 1, PositionX: 5.0, PositionY: 7.0},
		{Action: "Move", PlayerID: 1, PositionX: 5.0, PositionY: 8.0},
	}

	for i, msg := range messages {
		rawJSONBytes, _ := json.Marshal(msg)
		fmt.Printf("[客户端] 步数 %d -> 动作: %s, 目标坐标: X=%.0f, Y=%.0f | 塞入网卡的裸数据: %s\n",
			i+1, msg.Action, msg.PositionX, msg.PositionY, string(rawJSONBytes))

		sendRawJSON(conn, msg)
	}

	fmt.Println("[客户端] 3 条移动指令已全部快速发送完毕！(第一条立刻发出，后两条需要等待ack，Nagle算法才会合并发送)")
}

func sendRawJSON(conn net.Conn, msg any) {
	payload, _ := json.Marshal(msg)
	_, _ = conn.Write(payload)
}
