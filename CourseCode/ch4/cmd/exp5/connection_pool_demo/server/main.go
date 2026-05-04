package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9205", "监听地址")
	handshakeMS := flag.Int("handshake-ms", 30, "每条新连接的模拟建连/鉴权成本（毫秒）")
	workMS := flag.Int("work-ms", 3, "每个请求的服务端处理时间（毫秒）")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Printf("启动本机 TCP server 失败: %v\n", err)
		return
	}
	defer ln.Close()

	fmt.Println("=== 实验五：网络连接池服务端 ===")
	fmt.Printf("listen=%s, handshake=%dms, work=%dms\n", ln.Addr(), *handshakeMS, *workMS)
	fmt.Println("保持本终端运行，再在另一个终端启动 connection_pool_demo/client。")

	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleConn(conn, time.Duration(*handshakeMS)*time.Millisecond, time.Duration(*workMS)*time.Millisecond)
	}
}

func handleConn(conn net.Conn, handshakeCost, requestCost time.Duration) {
	defer conn.Close()

	time.Sleep(handshakeCost)

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				fmt.Printf("[server] read failed: %v\n", err)
			}
			return
		}

		time.Sleep(requestCost)
		if _, err := fmt.Fprintf(conn, "ok:%s", line); err != nil {
			return
		}
	}
}
