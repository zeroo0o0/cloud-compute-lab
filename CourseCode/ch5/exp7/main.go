package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func main() {
	id := flag.Int("id", 0, "节点 ID（必填，如 1、2、3）")
	port := flag.String("port", "", "HTTP 监听端口（必填，如 8001）")
	peersStr := flag.String("peers", "", "其他节点地址，逗号分隔（必填，如 localhost:8002,localhost:8003）")
	flag.Parse()

	if *id == 0 || *port == "" || *peersStr == "" {
		fmt.Println("用法: go run . -id <ID> -port <端口> -peers <其他节点地址>")
		fmt.Println("示例: go run . -id 1 -port 8001 -peers localhost:8002,localhost:8003")
		return
	}

	peers := strings.Split(*peersStr, ",")

	node := &Node{
		ID:           *id,
		state:        Follower,
		currentTerm:  0,
		votedFor:     -1,
		leaderID:     -1,
		peers:        peers,
		resetTimerCh: make(chan struct{}, 1),
	}

	// 注册 HTTP RPC handler
	http.HandleFunc("/request-vote", node.handleRequestVote)
	http.HandleFunc("/append-entries", node.handleAppendEntries)

	// 启动 HTTP server（后台运行）
	go func() {
		if err := http.ListenAndServe(":"+*port, nil); err != nil {
			log.Fatalf("HTTP server 启动失败: %v", err)
		}
	}()

	fmt.Printf("[Node %d][Term 0][Follower] started at :%s, peers=%v\n", *id, *port, peers)

	// 运行 Raft 状态机
	node.run()
}
