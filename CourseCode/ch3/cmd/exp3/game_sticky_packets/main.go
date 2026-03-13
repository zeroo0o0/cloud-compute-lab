package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"ch3/internal/ch3proto"
	"ch3/internal/ch3render"
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
	fmt.Println("====== 实验 3 ：Nagle 算法导致的半截粘包现场 =====")
	fmt.Println("==================================================")

	listener, err := net.Listen("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	go runServer(listener)
	time.Sleep(500 * time.Millisecond) // 等待服务器启动
	runClient()
	time.Sleep(2 * time.Second) // 等待所有 UI 渲染打印完毕
}

// 灾难版发送：没有任何边界保护，直接序列化后塞入网卡
func sendRawJSON(conn net.Conn, msg interface{}) {
	payload, _ := json.Marshal(msg)
	conn.Write(payload)
}

func runServer(listener net.Listener) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	// 1. 初始化游戏世界，玩家1出生在 (5, 5)
	ws := ch3proto.WorldState{
		Frame: 0,
		Players: []ch3proto.PlayerState{
			{ID: 1, X: 5, Y: 5, HP: 100, Online: true},
		},
		Event: "服务器就绪，等待玩家输入...",
	}

	// 渲染初始地图 UI
	fmt.Println("\n>>> [服务端] --- 游戏初始状态")
	fmt.Println(ch3render.FormatWorldState(ws, 28, 10))

	buffer := make([]byte, 1024)

	// ==========================================
	// 【阶段一】：瞬间读取第一个包
	// ==========================================
	n, err := conn.Read(buffer)
	if err == nil {
		var msg GameMessage
		err = json.Unmarshal(buffer[:n], &msg)
		ws.Frame++
		if err == nil {
			ws.Players[0].X = int(msg.PositionX)
			ws.Players[0].Y = int(msg.PositionY)
			ws.Event = "【Nagle放行】第1个包到达，解析成功，玩家移动！"

			// 【排版核心修改点】：底层虽然瞬间读到了数据，但我们刻意休眠 200ms 再打印。
			// 这样就能确保控制台上，客户端连发 3 条指令的日志能先一口气输出完毕！
			time.Sleep(200 * time.Millisecond)

			fmt.Println("\n>>> [服务端] --- 正常接收第 1 步 (Y=6)")
			fmt.Println(ch3render.FormatWorldState(ws, 28, 10))
		}
	}

	// 继续休眠 500ms，等待客户端 Nagle 算法积攒的第二、三个包（连体包）发送过来
	time.Sleep(500 * time.Millisecond)

	// ==========================================
	// 【阶段二】：读取 Nagle 拼车发来的后续连体包
	// ==========================================
	n, err = conn.Read(buffer)
	if err == nil {
		var msg GameMessage
		err = json.Unmarshal(buffer[:n], &msg)
		ws.Frame++
		if err != nil {
			// 灾难发生：解析失败，玩家状态无法更新，卡在 Y=6 原地！
			ws.Event = fmt.Sprintf("【Nagle拼车灾难】收到连体包 (%d 字节)，解析崩溃!", n)

			fmt.Println("\n>>> [服务端] --- 发生 TCP 粘包 (卡死在 Y=6)")
			fmt.Println(ch3render.FormatWorldState(ws, 28, 10))

			fmt.Println("\n[底层缓冲区残留的连体数据]：")
			fmt.Println(string(buffer[:n]))
		}
	}
}

func runClient() {
	conn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// 强行开启 Nagle 算法（延迟合并机制）
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(false)
	}

	fmt.Println("\n[客户端] 玩家快速连续按下了三次【向下移动】键,期望走到 Y=8 ...")

	// 【逻辑核心修改点】：改为向下移动，保持 X=5，增加 Y 的值
	messages := []GameMessage{
		{Action: "Move", PlayerID: 1, PositionX: 5.0, PositionY: 6.0},
		{Action: "Move", PlayerID: 1, PositionX: 5.0, PositionY: 7.0},
		{Action: "Move", PlayerID: 1, PositionX: 5.0, PositionY: 8.0},
	}

	// 瞬间循环发送 3 次
	for i, msg := range messages {
		rawJSONBytes, _ := json.Marshal(msg)
		fmt.Printf("[客户端] 步数 %d -> 动作: %s, 目标坐标: X=%.0f, Y=%.0f | 塞入网卡的裸数据: %s\n",
			i+1, msg.Action, msg.PositionX, msg.PositionY, string(rawJSONBytes))

		sendRawJSON(conn, msg)
	}

	fmt.Println("[客户端] 3 条移动指令已全部快速发送完毕！(第一条立刻发出，后两条需要等待ack，Nagle算法才会合并发送)")
}