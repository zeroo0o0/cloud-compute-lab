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
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "监听失败: %v\n", err)
		os.Exit(1)
	}
	defer ln.Close()
	fmt.Printf("🎮 BattleWorld 服务器启动，监听 %s\n等待 %d 名玩家...\n\n", addr, numPlayers)

	players := make([]*game.Player, 0, numPlayers)

	for len(players) < numPlayers {
		i := len(players)

		raw, err := ln.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "接受连接失败: %v\n", err)
			continue
		}

		conn := protocol.NewConn(raw)

		var name string
		valid := false

		// 🔥 循环直到拿到合法用户名
		for !valid {
			msg, err := conn.Receive()
			if err != nil || msg.Type != protocol.TypeJoin {
				fmt.Fprintf(os.Stderr, "等待 join 消息失败: %v\n", err)
				conn.Close()
				valid = false
				break // 直接跳出，重新 Accept
			}

			name = msg.Text
			if name == "" {
				name = fmt.Sprintf("玩家%d", i+1)
			}

			// 检查重名
			duplicate := false
			for _, existing := range players {
				if existing.Name == name {
					duplicate = true
					break
				}
			}

			if duplicate {
				conn.Send(protocol.Message{
					Type: protocol.TypeEvent,
					Text: "❌ 名字已被使用，请重新登录",
				})
				continue
			}

			valid = true
		}

		// ❌ 如果连接已经出错，跳过
		if !valid {
			continue
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
