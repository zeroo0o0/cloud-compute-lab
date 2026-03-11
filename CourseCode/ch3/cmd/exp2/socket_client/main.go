package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
)

func main() {
	host := "127.0.0.1"
	if len(os.Args) > 1 {
		host = os.Args[1]
	}
	conn, err := net.Dial("tcp", host+":9102")
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Println("connected", conn.RemoteAddr())

	stdin := bufio.NewReader(os.Stdin)
	netR := bufio.NewReader(conn)
	for {
		fmt.Print("输入发送内容(回车发送): ")
		line, _ := stdin.ReadString('\n')
		_, _ = conn.Write([]byte(line))
		resp, err := netR.ReadString('\n')
		if err != nil {
			fmt.Println("server closed:", err)
			return
		}
		fmt.Printf("resp: %s", resp)
	}
}
