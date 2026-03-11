package main

import (
	"fmt"
	"net"
	"time"
	"warzone/ch3/internal/ch3proto"
)

func server() {
	ln, err := net.Listen("tcp", ":9103")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	conn, err := ln.Accept()
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	for i := 0; i < 3; i++ {
		var m ch3proto.FrameMessage
		if err := ch3proto.RecvJSON(conn, &m); err != nil {
			fmt.Println("recv err:", err)
			return
		}
		fmt.Printf("server recv: %+v\n", m)
		_ = ch3proto.SendJSON(conn, ch3proto.FrameMessage{From: "server", Text: "ack:" + m.Text})
	}
}

func client() {
	time.Sleep(200 * time.Millisecond)
	conn, err := net.Dial("tcp", "127.0.0.1:9103")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	for i := 1; i <= 3; i++ {
		_ = ch3proto.SendJSON(conn, ch3proto.FrameMessage{From: "client", Text: fmt.Sprintf("msg-%d", i)})
		var resp ch3proto.FrameMessage
		if err := ch3proto.RecvJSON(conn, &resp); err != nil {
			fmt.Println("recv ack err:", err)
			return
		}
		fmt.Printf("client recv: %+v\n", resp)
	}
}

func main() {
	fmt.Println("=== Step3: 长度前缀 + JSON 解决粘包演示 ===")
	go server()
	client()
}
