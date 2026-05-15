package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"battleworld/protocol"
)

func main() {
	addr := os.Getenv("SERVER_ADDR")
	if addr == "" {
		addr = "127.0.0.1:9310"
	}

	fmt.Printf("=== Lab4 K8s 自动化测试 ===\n")
	fmt.Printf("服务器地址: %s\n\n", addr)

	tests := []struct {
		name string
		fn   func(string) error
	}{
		{"测试1: 注册与登录", testRegisterAndLogin},
		{"测试2: 获取世界状态", testWorldState},
	}

	passed := 0
	failed := 0

	for _, test := range tests {
		fmt.Printf("--- %s ---\n", test.name)
		if err := test.fn(addr); err != nil {
			fmt.Printf("  失败: %v\n", err)
			failed++
		} else {
			fmt.Printf("  通过\n")
			passed++
		}
	}

	fmt.Printf("\n=== 测试结果: %d 通过, %d 失败 ===\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func connect(addr string) (*protocol.Conn, error) {
	raw, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}
	return protocol.NewConn(raw), nil
}

func testRegisterAndLogin(addr string) error {
	conn, err := connect(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 注册
	if err := conn.Send(protocol.Message{
		Type:     protocol.TypeRegister,
		Username: "testuser",
		Password: "testpass",
		Confirm:  "testpass",
	}); err != nil {
		return fmt.Errorf("发送注册消息失败: %w", err)
	}

	reply, err := conn.Receive()
	if err != nil {
		return fmt.Errorf("接收注册结果失败: %w", err)
	}

	if reply.Type == protocol.TypeError {
		// 用户可能已存在，尝试登录
		fmt.Printf("  注册返回错误（可能已存在）: %s\n", reply.Error)
	} else if reply.OK {
		fmt.Printf("  注册成功\n")
	}

	return nil
}

func testWorldState(addr string) error {
	conn, err := connect(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 登录
	if err := conn.Send(protocol.Message{
		Type:     protocol.TypeLogin,
		Username: "testuser",
		Password: "testpass",
	}); err != nil {
		return fmt.Errorf("发送登录消息失败: %w", err)
	}

	reply, err := conn.Receive()
	if err != nil {
		return fmt.Errorf("接收登录结果失败: %w", err)
	}

	if reply.Type == protocol.TypeError {
		return fmt.Errorf("登录失败: %s", reply.Error)
	}

	if reply.State == nil {
		return fmt.Errorf("登录成功但状态为空")
	}

	state := reply.State
	fmt.Printf("  登录成功\n")
	fmt.Printf("  玩家: %s\n", state.Self.Username)
	fmt.Printf("  地图: %s (%s)\n", state.Map.Name, state.Map.NodeID)
	fmt.Printf("  节点数: %d\n", len(state.Nodes))
	fmt.Printf("  地图数: %d\n", len(state.Maps))

	// 验证状态
	if state.Self.Username != "testuser" {
		return fmt.Errorf("用户名不匹配: got %s", state.Self.Username)
	}

	if len(state.Nodes) == 0 {
		return fmt.Errorf("没有节点信息")
	}

	if len(state.Maps) == 0 {
		return fmt.Errorf("没有地图信息")
	}

	// 打印详细信息
	data, _ := json.MarshalIndent(state, "  ", "  ")
	fmt.Printf("  状态详情:\n  %s\n", string(data))

	return nil
}
