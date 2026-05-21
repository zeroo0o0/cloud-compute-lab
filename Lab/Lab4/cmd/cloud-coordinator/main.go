package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"battleworld/cloud"
	"battleworld/cluster"
	"battleworld/storage"
)

func main() {
	dataRoot := defaultEnv("LAB4_DATA_ROOT", ".")
	listenAddr := defaultEnv("LAB4_COORDINATOR_ADDR", ":9320")

	var store cloud.Store
	var elector *cluster.LeaderElector
	if strings.EqualFold(defaultEnv("LAB4_KUBE_STATE", "false"), "true") {
		client, err := cluster.NewInClusterClient(defaultEnv("LAB4_NAMESPACE", ""))
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化 Kubernetes 状态客户端失败：%v\n", err)
			os.Exit(1)
		}
		store = storage.NewKubeStore(client, defaultEnv("LAB4_COORDINATOR_STATE", "lab4-coordinator-state"))
		elector = cluster.NewLeaderElector(client, defaultEnv("LAB4_COMPONENT", "coordinator"), listenAddr)
		elector.Start(context.Background())
	} else {
		fileStore, err := cloud.NewDefaultStore(dataRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化存储失败：%v\n", err)
			os.Exit(1)
		}
		store = fileStore
	}

	maps := map[string]*cloud.MapClient{
		"green": cloud.NewMapClient("green", defaultEnv("LAB4_GREEN_NODE_ID", "map-green"), requiredEnv("LAB4_GREEN_URL")),
		"cave":  cloud.NewMapClient("cave", defaultEnv("LAB4_CAVE_NODE_ID", "map-cave"), requiredEnv("LAB4_CAVE_URL")),
		"ruins": cloud.NewMapClient("ruins", defaultEnv("LAB4_RUINS_NODE_ID", "map-ruins"), requiredEnv("LAB4_RUINS_URL")),
	}

	coordinator := cloud.NewCoordinator(store, maps)
	if elector != nil {
		coordinator.SetActivePredicate(elector.IsLeader)
	}
	coordinator.Start()

	handler := coordinator.Handler()
	if elector != nil {
		handler = elector.ProxyOrServe(handler)
	}

	fmt.Printf("协调器已启动：listen=%s green=%s cave=%s ruins=%s\n", listenAddr, requiredEnv("LAB4_GREEN_URL"), requiredEnv("LAB4_CAVE_URL"), requiredEnv("LAB4_RUINS_URL"))
	if err := http.ListenAndServe(listenAddr, handler); err != nil {
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
