package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	// 创建 TCP 监听
	listener, err := net.Listen("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	fmt.Println("[服务器] 启动成功，等待玩家连接...")

	// 接收玩家1
	conn1, _ := listener.Accept()
	fmt.Printf("[服务器] 玩家1连接: %s\n", conn1.RemoteAddr().String())

	// 接收玩家2
	conn2, _ := listener.Accept()
	fmt.Printf("[服务器] 玩家2连接: %s\n", conn2.RemoteAddr().String())

	// 等待玩家1发起攻击的指令
	buf := make([]byte, 1024)
	conn1.Read(buf)
	fmt.Println("[服务器] 玩家1发起攻击，玩家2HP归零...")

	// 准备广播的状态数据 (精确的23字节，末尾补了几个空格)
	msg := []byte("STATE: Player2 DEAD    ")

	// 发送给玩家1
	_, err1 := conn1.Write(msg)
	fmt.Printf("[服务器] 发送给玩家1: 23 bytes, err=%v  <- 成功\n", err1)

	// 发送给玩家2
	// 核心演示点：玩家2处于休眠(卡死)状态，不去读数据。
	// 但只要内核发送缓冲区未满，Write 依然会瞬间返回 nil。
	_, err2 := conn2.Write(msg)
	fmt.Printf("[服务器] 发送给玩家2: 23 bytes, err=%v  <- 也\"成功\"！(实际进内核缓冲区了)\n", err2)

	fmt.Println("[服务器] 广播完成，继续游戏...")
	
	// 保持服务器运行一会儿，让输出可见
	time.Sleep(2 * time.Second)
}