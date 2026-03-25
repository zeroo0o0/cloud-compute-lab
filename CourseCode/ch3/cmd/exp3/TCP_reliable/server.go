package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	fmt.Println("[服务器] 启动成功，开始监听端口 8888...")

	// 1. 接收玩家1
	conn1, _ := listener.Accept()
	fmt.Printf("[服务器] 玩家1连接: %s (初始HP: 100)\n", conn1.RemoteAddr().String())

	// 2. 接收玩家2
	conn2, _ := listener.Accept()
	fmt.Printf("[服务器] 玩家2连接: %s (初始HP: 100)\n", conn2.RemoteAddr().String())

	// 3. 核心修改：通知玩家1，玩家2已经上线，可以开始攻击了
	conn1.Write([]byte("P2_ONLINE"))

	// 4. 等待玩家1发起攻击的指令
	buf := make([]byte, 1024)
	conn1.Read(buf)
	fmt.Println("[服务器] 接收到玩家1攻击指令，判定玩家2 HP 归零...")

	// 稍微等待1秒，确保玩家2的终端已经真实断开底层连接
	time.Sleep(1 * time.Second)

	msg := []byte("STATE: Player2 DEAD    ")

	// 发送给玩家1
	_, err1 := conn1.Write(msg)
	fmt.Printf("[服务器] 发送给玩家1: 23 bytes, err=%v  <- 成功\n", err1)

	// 发送给已经断线的玩家2
	_, err2 := conn2.Write(msg)
	fmt.Printf("[服务器] 发送给玩家2: 23 bytes, err=%v  <- 也\"成功\"！\n", err2)

	fmt.Println("[服务器] 广播完成。")
	fmt.Println("--------------------------------------------------")
	fmt.Println("[服务器] 进入持续监听状态，等待断线玩家唤醒并重连...")

	// 无限循环，保持一直 listen 的状态
	for {
		conn3, err := listener.Accept()
		if err != nil {
			continue
		}
		
		fmt.Printf("\n[服务器] 收到新连接: %s (检测到玩家重连)\n", conn3.RemoteAddr().String())
		fmt.Println("[服务器] 严重状态冲突：服务器内存中该玩家已死，但新连接的客户端依然满血！")
		
		// 可以在这里回复重连成功的消息，保持连接存活
		conn3.Write([]byte("WELCOME_BACK"))
	}
}