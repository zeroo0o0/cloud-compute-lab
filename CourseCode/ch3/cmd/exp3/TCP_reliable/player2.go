package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	fmt.Println("[玩家2] 突然断网 (网线被拔/进程卡死)！")
	fmt.Println("# (玩家2永远收不到\"DEAD\"消息)")


	time.Sleep(5 * time.Second)

	fmt.Println("[玩家2] 重启（以为自己满血）！")
}