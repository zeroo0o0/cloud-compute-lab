package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	fmt.Println("=======================================")
	fmt.Println("         交互式网络客户端启动          ")
	fmt.Println("=======================================")

	// 【核心改动 2】：让用户选择是单机测试还是多机测试
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(">> 请输入服务器IP (单机测试请直接按回车，跨机请输入局域网IP): ")
	ipInput, _ := reader.ReadString('\n')
	ipInput = strings.TrimSpace(ipInput)
	
	// 如果用户直接按了回车，默认使用本机回环地址
	if ipInput == "" {
		ipInput = "127.0.0.1"
	}

	targetAddress := ipInput + ":8888"
	fmt.Printf("\n[系统] 正在连接到 %s ...\n", targetAddress)

	conn, err := net.Dial("tcp", targetAddress)
	if err != nil {
		fmt.Println("连接失败，请检查 IP 或服务器是否开启:", err)
		return
	}
	defer conn.Close()
	fmt.Println("=> 连接成功！现在您可以自由输入内容了 (输入 exit 退出)。")

	// 【核心改动 3】：捕捉终端的真实键盘输入
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("请输入要发送的指令/文本: ")
		
		// 阻塞等待用户在终端敲击回车
		if !scanner.Scan() {
			break
		}
		
		text := strings.TrimSpace(scanner.Text())
		
		// 退出机制
		if strings.ToLower(text) == "exit" {
			fmt.Println("[系统] 主动断开连接。")
			break
		}
		
		// 防呆设计：不发送空字符串
		if text == "" {
			continue
		}

		// 将真实的文本转换为字节流发送给服务器
		_, err = conn.Write([]byte(text))
		if err != nil {
			fmt.Println("发送失败:", err)
			break
		}

		// 阻塞等待并读取服务器的应答
		buffer := make([]byte, 1024)
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Println("读取服务器应答失败:", err)
			break
		}

		fmt.Printf("   <- [收到服务器回执]: %s\n\n", string(buffer[:n]))
	}
}