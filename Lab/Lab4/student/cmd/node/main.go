package main

import (
	"fmt"
	"os"
	"strings"

	"battleworld/node"
	"battleworld/world"
)

// 游戏节点独立入口（微服务模式）
// 环境变量:
//   ROLE=node
//   NODE_ID=node-a
//   NODE_ADDR=0.0.0.0:9311
//   MAP_ASSIGNMENTS=green=node-a:node-c,...
//   LAB3_DATA_ROOT=/data

func main() {
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		fmt.Fprintf(os.Stderr, "[node] NODE_ID 环境变量未设置\n")
		os.Exit(1)
	}

	nodeAddr := os.Getenv("NODE_ADDR")
	if nodeAddr == "" {
		// 根据 nodeID 推断默认端口
		switch nodeID {
		case "node-a":
			nodeAddr = "0.0.0.0:9311"
		case "node-b":
			nodeAddr = "0.0.0.0:9312"
		case "node-c":
			nodeAddr = "0.0.0.0:9313"
		default:
			nodeAddr = "0.0.0.0:9311"
		}
	}

	// 如果指定了 POD_NAME，从中推断 NODE_ID
	if podName := os.Getenv("POD_NAME"); podName != "" {
		// StatefulSet 的 Pod 名称格式: battleworld-node-0, battleworld-node-1, ...
		if strings.HasSuffix(podName, "-0") {
			nodeID = "node-a"
		} else if strings.HasSuffix(podName, "-1") {
			nodeID = "node-b"
		} else if strings.HasSuffix(podName, "-2") {
			nodeID = "node-c"
		}
	}

	fmt.Fprintf(os.Stderr, "[node] 启动节点: %s, 地址: %s\n", nodeID, nodeAddr)

	server := node.NewServer(nodeID, nodeAddr)

	// 安装地图
	assignments := os.Getenv("MAP_ASSIGNMENTS")
	if assignments == "" {
		assignments = "green=node-a:node-c,cave=node-b:node-c,ruins=node-a:node-b"
	}

	mapsToInstall := make(map[string]bool)
	for _, pair := range strings.Split(assignments, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			mapID := parts[0]
			nodes := strings.SplitN(parts[1], ":", 2)
			for _, n := range nodes {
				if n == nodeID {
					mapsToInstall[mapID] = true
				}
			}
		}
	}

	for _, cfg := range world.AvailableMaps() {
		if mapsToInstall[cfg.ID] {
			server.InstallMap(cfg)
			fmt.Fprintf(os.Stderr, "[node] 已安装地图: %s (%s)\n", cfg.ID, cfg.Name)
		}
	}

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "[node] 启动失败: %v\n", err)
		os.Exit(1)
	}

	// 阻塞主线程
	select {}
}
