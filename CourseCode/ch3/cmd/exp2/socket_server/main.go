package main

import (
	"fmt"
	"net"
)

func main() {
	// 【核心改动 1】：使用 ":8888" (等同于 "0.0.0.0:8888")
	// 这使得服务器同时兼容单机测试 (127.0.0.1) 和 多机局域网测试 (如 192.168.x.x)
	listener, err := net.Listen("tcp", ":8888")
	if err != nil {
		fmt.Println("启动服务器失败:", err)
		return
	}
	defer listener.Close()

	fmt.Println("=======================================")
	fmt.Println("  通用交互式服务器已启动 (端口: 8888)  ")
	fmt.Println("  支持单机 Localhost 与 局域网跨机连接 ")
	fmt.Println("=======================================")

	for {
		fmt.Println("\n[系统] 等待客户端接入...")
		conn, err := listener.Accept()
		if err != nil {
			continue
		}

		fmt.Printf("=> 成功！新客户端已接入 (IP: %s)\n", conn.RemoteAddr().String())
		
		// 进入连接处理逻辑
		handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	buffer := make([]byte, 1024)

	// 使用 for 循环保持长连接，持续等待客户端的多次输入
	for {
		// 这里会阻塞，静静等待客户端通过键盘输入并发送数据
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Printf("[系统] 客户端 %s 已断开连接。\n", conn.RemoteAddr().String())
			return // 发生错误或客户端断开时，结束当前连接的循环
		}

		// 解析真实输入的内容
		receivedMsg := string(buffer[:n])
		fmt.Printf("   -> [收到客户端消息]: %s\n", receivedMsg)

		// 给出应答
		replyMsg := "服务器已阅: [" + receivedMsg + "]"
		conn.Write([]byte(replyMsg))
	}
}