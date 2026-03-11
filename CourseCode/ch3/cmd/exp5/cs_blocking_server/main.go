package main

import (
	"bufio"
	"fmt"
	"net"
	"time"
)

// 单线程阻塞服务器：一次只处理一个客户端，其余排队等待
func handleClient(conn net.Conn, id int) {
	defer conn.Close()
	fmt.Printf("[blocking] 开始处理 client#%d %s\n", id, conn.RemoteAddr())
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			fmt.Printf("[blocking] client#%d 断开: %v\n", id, err)
			return
		}
		fmt.Printf("[blocking] client#%d: %s", id, line)
		reply := fmt.Sprintf("[server→client#%d] echo: %s", id, line)
		conn.Write([]byte(reply))
	}
}

func main() {
	ln, err := net.Listen("tcp", ":9105")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	fmt.Println("=== Step5 阻塞服务器 :9105 ===")
	fmt.Println("特点：同一时刻只处理一个连接，后续连接必须排队等待前一个完成")
	fmt.Println("演示方法：开3个client窗口连接，观察只有第1个能交互")
	fmt.Println()

	busy := false
	id := 0
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		id++
		if busy {
			fmt.Printf("[blocking] client#%d 尝试连接，但服务器正忙：%s\n", id, conn.RemoteAddr())
			_, _ = conn.Write([]byte("[server] 当前阻塞服务器正在处理前一个客户端，你正在排队中，请稍后重试。\n"))
			_ = conn.Close()
			continue
		}
		busy = true
		fmt.Printf("[blocking] 接受连接 #%d from %s\n", id, conn.RemoteAddr())
		// *** 关键：直接调用而不是 go，导致阻塞 ***
		handleClient(conn, id)
		busy = false
		// 只有这个客户端断开后才能 Accept 下一个
		time.Sleep(100 * time.Millisecond)
	}
}
