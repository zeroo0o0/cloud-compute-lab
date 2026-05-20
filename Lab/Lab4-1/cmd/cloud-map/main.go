package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"battleworld/cloud"
)

func main() {
	mapID := requiredEnv("LAB3_MAP_ID")
	nodeID := defaultEnv("LAB3_NODE_ID", "map-"+mapID)
	listenAddr := defaultEnv("LAB3_MAP_LISTEN_ADDR", ":9400")

	service, err := cloud.NewMapService(nodeID, mapID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化地图服务失败：%v\n", err)
		os.Exit(1)
	}
	stop := make(chan struct{})
	service.StartBackground(stop)

	fmt.Printf("地图服务已启动：map=%s node=%s listen=%s\n", mapID, nodeID, listenAddr)
	if err := http.ListenAndServe(listenAddr, service.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "地图服务监听失败：%v\n", err)
		os.Exit(1)
	}
}

func requiredEnv(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		fmt.Fprintf(os.Stderr, "缺少环境变量 %s\n", key)
		os.Exit(1)
	}
	return value
}

func defaultEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
