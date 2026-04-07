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
		// 方案 A：每次等待新连接前，先清理大厅里已经掉线的玩家。
		players = sweepDisconnectedPlayers(players)

		raw, err := ln.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "接受连接失败: %v\n", err)
			continue
		}

		conn := protocol.NewConn(raw)
		name, ok := negotiateJoinName(conn, players)
		if !ok {
			conn.Close()
			continue
		}

		// 名字协商完成后，再清理一次大厅中的失效连接，避免断线玩家继续占名额。
		players = sweepDisconnectedPlayers(players)
		i := len(players)

		startX, startY := 0, 0
		if i == 1 {
			startX = protocol.MapWidth - 1
			startY = protocol.MapHeight - 1
		}

		p := game.NewPlayer(i+1, name, startX, startY, conn)
		players = append(players, p)

		fmt.Printf("✅ 玩家 %d [%s] 已连接 → 起始位置 (%d,%d)\n", i+1, name, startX, startY)

		if err := conn.Send(protocol.Message{
			Type:   protocol.TypeInit,
			YourID: i + 1,
			Text:   fmt.Sprintf("欢迎，%s！等待对手...", name),
		}); err != nil {
			fmt.Fprintf(os.Stderr, "发送 init 失败: %v\n", err)
			conn.Close()
			players = players[:len(players)-1]
			continue
		}
	}

	if !checkPlayersBeforeStart(players) {
		fmt.Println("⚠️ 开始前检测到有玩家掉线，已取消本轮匹配，等待重新连接...\n")
		closePlayers(players, "⚠️ 开始前检测到掉线，本轮匹配已取消，请重新连接。")
		return
	}

	fmt.Println("\n🚀 双方就绪，游戏开始！\n")
	g := game.NewGame(players[0], players[1])
	g.Run()
	fmt.Println("\n🏁 本局结束，服务器退出。")
}

// negotiateJoinName 负责在同一条连接上完成名字协商。
// 若名字重复，则提示客户端重新输入名字，而不是直接关闭连接。
func negotiateJoinName(conn *protocol.Conn, players []*game.Player) (string, bool) {
	for {
		msg, err := conn.Receive()
		if err != nil {
			fmt.Fprintf(os.Stderr, "等待 join 消息失败: %v\n", err)
			return "", false
		}
		if msg.Type != protocol.TypeJoin {
			_ = conn.Send(protocol.Message{
				Type: protocol.TypeEvent,
				Text: "❌ 非法登录消息，请重新输入名字。",
			})
			continue
		}

		name := msg.Text
		if name == "" {
			name = fmt.Sprintf("玩家%d", len(players)+1)
		}

		// 每次做重名判断前，都先扫掉线，避免断线玩家继续占用名字。
		players = sweepDisconnectedPlayers(players)

		duplicate := false
		for _, existing := range players {
			if existing.Name == name {
				duplicate = true
				break
			}
		}
		if duplicate {
			_ = conn.Send(protocol.Message{
				Type: protocol.TypeEvent,
				Text: "❌ 名字已被使用，请重新输入名字。",
			})
			continue
		}

		return name, true
	}
}

// sweepDisconnectedPlayers 清理大厅中已经断开的玩家。
// 实现方式：给等待中的玩家发送一条探测消息；发送失败则说明连接已失效。
func sweepDisconnectedPlayers(players []*game.Player) []*game.Player {
	kept := make([]*game.Player, 0, len(players))

	for _, p := range players {
		if p == nil || p.Conn == nil {
			continue
		}

		err := p.Conn.Send(protocol.Message{
			Type: protocol.TypeEvent,
			Text: "⏳ 仍在等待其他玩家加入...",
		})
		if err != nil {
			fmt.Printf("⚠️ 玩家 [%s] 在大厅等待阶段已掉线，已移除\n", p.Name)
			p.Conn.Close()
			continue
		}

		kept = append(kept, p)
	}

	return kept
}

// checkPlayersBeforeStart 在正式开局前做最后一次连接检查。
func checkPlayersBeforeStart(players []*game.Player) bool {
	for _, p := range players {
		if p == nil || p.Conn == nil {
			return false
		}
		if err := p.Conn.Send(protocol.Message{
			Type: protocol.TypeEvent,
			Text: "✅ 连接检查通过，正在进入对局...",
		}); err != nil {
			fmt.Fprintf(os.Stderr, "玩家 [%s] 开始前连接检查失败: %v\n", p.Name, err)
			return false
		}
	}
	return true
}

// closePlayers 在匹配取消时显式关闭当前持有的连接资源。
func closePlayers(players []*game.Player, notice string) {
	for _, p := range players {
		if p == nil || p.Conn == nil {
			continue
		}
		if notice != "" {
			_ = p.Conn.Send(protocol.Message{Type: protocol.TypeEvent, Text: notice})
		}
		p.Conn.Close()
	}
}
