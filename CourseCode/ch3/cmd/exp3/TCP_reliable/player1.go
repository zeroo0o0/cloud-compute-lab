package main

import (
	"fmt"
	"net"
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	hp := 100
	fmt.Printf("[玩家1] 成功上线，当前血量: %d HP\n", hp)
	fmt.Println("[玩家1] 正在等待玩家2上线...")

	// 核心修改：阻塞等待服务器发送 P2_ONLINE 信号
	signalBuf := make([]byte, 9)
	conn.Read(signalBuf)

	if string(signalBuf) == "P2_ONLINE" {
		fmt.Println("[玩家1] 发现玩家2已上线！")
		// 严格保证在玩家2上线后才执行这句
		fmt.Println("[玩家1] 对玩家2发起致命一击！")
		conn.Write([]byte("ATTACK"))
	}

	// 接收服务器广播的死亡状态
	buf := make([]byte, 23)
	conn.Read(buf)
	fmt.Printf("[玩家1] 收到状态: %s\n", string(buf))
	fmt.Println("[玩家1] 确认: 玩家2已死亡")
}