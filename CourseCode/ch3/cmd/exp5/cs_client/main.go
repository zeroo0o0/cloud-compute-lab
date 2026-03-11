package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	port := "9106"
	host := "127.0.0.1"
	if len(os.Args) > 1 {
		host = os.Args[1]
	}
	if len(os.Args) > 2 {
		port = os.Args[2]
	}
	addr := host + ":" + port
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Printf("Step5 client connected to %s\n", addr)
	fmt.Println("用法: go run . [host] [port]")
	fmt.Println("  连阻塞服务器: go run . 127.0.0.1 9105")
	fmt.Println("  连并发服务器: go run . 127.0.0.1 9106")
	fmt.Println()

	stdin := bufio.NewReader(os.Stdin)
	netR := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if msg, err := netR.ReadString('\n'); err == nil {
		fmt.Print(msg)
		if port == "9105" {
			fmt.Println("当前连接的是阻塞服务器；若收到排队提示，说明服务端正在忙。")
			return
		}
	}
	_ = conn.SetReadDeadline(time.Time{})
	for {
		fmt.Print("> ")
		line, _ := stdin.ReadString('\n')
		if _, err := conn.Write([]byte(line)); err != nil {
			fmt.Println("send err:", err)
			return
		}
		resp, err := netR.ReadString('\n')
		if err != nil {
			fmt.Println("server closed:", err)
			return
		}
		fmt.Print(resp)
	}
}
