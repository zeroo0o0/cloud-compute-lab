package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
)

func main() {
	// 第一次连接
	conn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		panic(err)
	}

	hp := 100
	fmt.Printf("[玩家2] 首次上线成功，当前血量: %d HP\n", hp)

	fmt.Println("[玩家2] 突然断网 (切断连接)！")
	
	// 发生真实断线：直接关闭底层 Socket
	conn.Close()

	// 阻塞终端，等待讲师手动输入回车
	fmt.Print("\n>>> 请在终端按下回车键，模拟玩家2唤醒重启客户端 <<<")
	reader := bufio.NewReader(os.Stdin)
	reader.ReadBytes('\n')

	// 唤醒后：发起全新的 TCP 连接
	fmt.Println("\n[玩家2] 客户端唤醒，正在重新连接服务器...")
	reconnectConn, err := net.Dial("tcp", "127.0.0.1:8888")
	if err != nil {
		fmt.Printf("[玩家2] 重连失败: %v\n", err)
		return
	}
	defer reconnectConn.Close()

	// 客户端本地状态重置
	fmt.Printf("[玩家2] 重连成功！当前血量: %d HP\n", hp)
	
	
	// 保持进程存活，接收服务端的欢迎消息
	welcomeBuf := make([]byte, 12)
	reconnectConn.Read(welcomeBuf)
}