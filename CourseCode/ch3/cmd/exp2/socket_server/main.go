package main

import (
	"bufio"
	"fmt"
	"net"
)

func main() {
	ln, err := net.Listen("tcp", ":9102")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	fmt.Println("Step2 server listening on :9102")

	conn, err := ln.Accept()
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Println("client connected:", conn.RemoteAddr())

	r := bufio.NewReader(conn)
	for {
		msg, err := r.ReadString('\n')
		if err != nil {
			fmt.Println("read end:", err)
			return
		}
		fmt.Printf("recv: %s", msg)
		_, _ = conn.Write([]byte("server ack: " + msg))
	}
}
