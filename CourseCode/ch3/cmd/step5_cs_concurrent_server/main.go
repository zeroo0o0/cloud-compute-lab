package main

import (
	"bufio"
	"fmt"
	"net"
	"sync"
)

// 并发服务器：用 go handleClient 为每个连接开独立 goroutine
func handleClient(conn net.Conn, id int, mu *sync.Mutex) {
	defer conn.Close()
	mu.Lock()
	fmt.Printf("[concurrent] goroutine 启动 → client#%d %s\n", id, conn.RemoteAddr())
	mu.Unlock()

	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			mu.Lock()
			fmt.Printf("[concurrent] client#%d 断开: %v\n", id, err)
			mu.Unlock()
			return
		}
		// mu.Lock()
		fmt.Printf("[concurrent] client#%d: %s", id, line)
		// mu.Unlock()
		reply := fmt.Sprintf("[server→client#%d] echo: %s", id, line)
		conn.Write([]byte(reply))
	}
}

func main() {
	ln, err := net.Listen("tcp", ":9106")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	fmt.Println("=== Step5 并发服务器 :9106 ===")
	fmt.Println("特点：每个新连接用 go handleClient() 开 goroutine，互不阻塞")
	fmt.Println("演示方法：开3个client窗口同时连接，观察全部都可以交互")
	fmt.Println()

	var mu sync.Mutex
	id := 0
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		id++
		fmt.Printf("[concurrent] 接受连接 #%d from %s\n", id, conn.RemoteAddr())
		// *** 关键：go 开启 goroutine，不阻塞 Accept 循环 ***
		go handleClient(conn, id, &mu)
	}
}
