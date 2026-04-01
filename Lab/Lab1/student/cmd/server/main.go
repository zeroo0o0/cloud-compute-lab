// server 是 BattleWorld Lab1 的服务端程序。
//
// 架构：C/S（Client/Server）
//   - 服务器顺序接受恰好 2 个 TCP 连接
//   - 接受完毕后将连接交给 game.Game 进行回合制主循环
//   - 所有游戏逻辑在服务端执行（服务端权威模型）
//   - 客户端仅负责用户 I/O 和消息收发
//
// 启动方式：
//   go run ./cmd/server
package main

import (
	"fmt"
	"net"
	"os"

	"battleworld/game"
	"battleworld/protocol"
)

const (
	addr       = ":9000"
	numPlayers = 2
)

func main() {
	// ╔═════════════════════════════════════════════════════════════════════════╗
	// ║  任务 C-1：启动 TCP 监听                                               ║
	// ║                                                                         ║
	// ║  功能：在指定地址上开启 TCP 监听，等待客户端连接。                      ║
	// ║                                                                         ║
	// ║  实现要点：                                                             ║
	// ║    调用 net.Listen("tcp", addr)，将返回值存入 ln 和 err。               ║
	// ║    若 err != nil，向 os.Stderr 打印错误信息后 os.Exit(1)。              ║
	// ║    使用 defer ln.Close() 确保程序退出时关闭监听器。                     ║
	// ║                                                                         ║
	// ║  提示框架：                                                             ║
	// ║    ln, err := net.Listen(???, ???)                                      ║
	// ║    if err != nil { ... }                                                ║
	// ║    defer ln.Close()                                                     ║
	// ╚═════════════════════════════════════════════════════════════════════════╝

	// TODO C-1: 在此处启动 TCP 监听
	panic("C-1 尚未实现：请完成 net.Listen 调用")

	fmt.Printf("🎮 BattleWorld 服务器启动，监听 %s\n等待 %d 名玩家...\n\n", addr, numPlayers)

	players := make([]*game.Player, 0, numPlayers)

	for len(players) < numPlayers {
		i := len(players)

		// ╔═════════════════════════════════════════════════════════════════════════╗
		// ║  任务 C-2：接受客户端连接                                             ║
		// ║                                                                         ║
		// ║  功能：从监听器接受一个新的 TCP 连接。                                  ║
		// ║                                                                         ║
		// ║  实现要点：                                                             ║
		// ║    调用 ln.Accept()，将返回值存入 raw 和 err。                          ║
		// ║    若 err != nil，打印错误后 continue（跳过本次，继续等待下一个连接）。  ║
		// ║                                                                         ║
		// ║  提示框架：                                                             ║
		// ║    raw, err := ln.Accept()                                              ║
		// ║    if err != nil { ...; continue }                                      ║
		// ╚═════════════════════════════════════════════════════════════════════════╝

		// TODO C-2: 在此处接受客户端连接
		panic("C-2 尚未实现：请完成 ln.Accept() 调用")

		conn := protocol.NewConn(raw)

		msg, err := conn.Receive()
		if err != nil || msg.Type != protocol.TypeJoin {
			fmt.Fprintf(os.Stderr, "等待 join 消息失败: %v\n", err)
			conn.Close()
			continue
		}

		name := msg.Text
		if name == "" {
			name = fmt.Sprintf("玩家%d", i+1)
		}

		startX, startY := 0, 0
		if i == 1 {
			startX = protocol.MapWidth - 1
			startY = protocol.MapHeight - 1
		}

		p := game.NewPlayer(i+1, name, startX, startY, conn)
		players = append(players, p)

		fmt.Printf("✅ 玩家 %d [%s] 已连接 → 起始位置 (%d,%d)\n", i+1, name, startX, startY)

		conn.Send(protocol.Message{
			Type:   protocol.TypeInit,
			YourID: i + 1,
			Text:   fmt.Sprintf("欢迎，%s！等待对手...", name),
		})
	}

	fmt.Println("\n🚀 双方就绪，游戏开始！\n")
	g := game.NewGame(players[0], players[1])
	g.Run()
	fmt.Println("\n🏁 本局结束，服务器退出。")
}