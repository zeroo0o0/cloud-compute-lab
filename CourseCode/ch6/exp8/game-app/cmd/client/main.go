package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func runOnce(addr, token, player string) error {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// 重连时仍然发送同一个 token，gateway 才能从 Redis 恢复原 session。
	if _, err := fmt.Fprintf(conn, "HELLO %s %s\n", token, player); err != nil {
		return err
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	log.Printf("[client] %s", strings.TrimSpace(line))

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		// 长连接存活期间持续心跳；连接断开时读写会报错并退出 runOnce。
		if _, err := fmt.Fprintf(conn, "HEARTBEAT %s\n", token); err != nil {
			return err
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		log.Printf("[client] %s", strings.TrimSpace(line))
	}
	return nil
}

func main() {
	addr := env("GATEWAY_ADDR", "127.0.0.1:8080")
	token := env("TOKEN", "student-1-token")
	player := env("PLAYER_ID", "student-1")

	for {
		// 自动重连循环：旧 Gateway-Pod 被删后，下一次连接会打到其他可用 Pod。
		log.Printf("[client] connecting addr=%s token=%s", addr, token)
		if err := runOnce(addr, token, player); err != nil {
			log.Printf("[client] disconnected: %v", err)
			time.Sleep(time.Second)
		}
	}
}
