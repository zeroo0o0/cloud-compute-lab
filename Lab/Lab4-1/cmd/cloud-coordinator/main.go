package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"battleworld/cloud"
)

func main() {
	dataRoot := defaultEnv("LAB3_DATA_ROOT", ".")
	listenAddr := defaultEnv("LAB3_COORDINATOR_ADDR", ":9320")

	store, err := cloud.NewDefaultStore(dataRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化存储失败：%v\n", err)
		os.Exit(1)
	}

	maps := map[string]*cloud.MapClient{
		"green": cloud.NewMapClient("green", defaultEnv("LAB3_GREEN_NODE_ID", "map-green"), requiredEnv("LAB3_GREEN_URL")),
		"cave":  cloud.NewMapClient("cave", defaultEnv("LAB3_CAVE_NODE_ID", "map-cave"), requiredEnv("LAB3_CAVE_URL")),
		"ruins": cloud.NewMapClient("ruins", defaultEnv("LAB3_RUINS_NODE_ID", "map-ruins"), requiredEnv("LAB3_RUINS_URL")),
	}

	coordinator := cloud.NewCoordinator(store, maps)
	coordinator.Start()

	fmt.Printf("协调器已启动：listen=%s green=%s cave=%s ruins=%s\n", listenAddr, requiredEnv("LAB3_GREEN_URL"), requiredEnv("LAB3_CAVE_URL"), requiredEnv("LAB3_RUINS_URL"))
	if err := http.ListenAndServe(listenAddr, coordinator.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "协调器监听失败：%v\n", err)
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
