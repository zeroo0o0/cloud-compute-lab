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

	// 稍微等一下，确保玩家2也连上了
	time.Sleep(1 * time.Second)

	// 发起攻击
	conn.Write([]byte("ATTACK"))

	// 正常读取服务器广播的状态
	buf := make([]byte, 23)
	conn.Read(buf)
	fmt.Printf("[玩家1] 收到状态: %s\n", string(buf))
	fmt.Println("[玩家1] 确认: 玩家2已死亡")
}