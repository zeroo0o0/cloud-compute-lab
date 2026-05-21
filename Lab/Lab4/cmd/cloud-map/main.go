package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"battleworld/cloud"
	"battleworld/cluster"
	"battleworld/protocol"
)

func main() {
	mapID := requiredEnv("LAB4_MAP_ID")
	nodeID := defaultEnv("LAB4_NODE_ID", "map-"+mapID)
	listenAddr := defaultEnv("LAB4_MAP_LISTEN_ADDR", ":9400")

	service, err := cloud.NewMapService(nodeID, mapID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化地图服务失败：%v\n", err)
		os.Exit(1)
	}

	var elector *cluster.LeaderElector
	var kubeClient *cluster.Client
	if strings.EqualFold(defaultEnv("LAB4_KUBE_STATE", "false"), "true") {
		kubeClient, err = cluster.NewInClusterClient(defaultEnv("LAB4_NAMESPACE", ""))
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化 Kubernetes 状态客户端失败：%v\n", err)
			os.Exit(1)
		}
		checkpointName := defaultEnv("LAB4_MAP_CHECKPOINT", "lab4-map-"+mapID+"-checkpoint")
		restoreCheckpoint(kubeClient, checkpointName, service)
		elector = cluster.NewLeaderElector(kubeClient, defaultEnv("LAB4_COMPONENT", "map-"+mapID), listenAddr)
		elector.Start(context.Background())
		go checkpointLoop(context.Background(), kubeClient, checkpointName, service, elector)
	}

	stop := make(chan struct{})
	if elector != nil {
		service.StartBackgroundWhen(stop, elector.IsLeader)
	} else {
		service.StartBackground(stop)
	}

	handler := service.Handler()
	if elector != nil {
		handler = elector.ProxyOrServe(handler)
	}

	fmt.Printf("地图服务已启动：map=%s node=%s listen=%s\n", mapID, nodeID, listenAddr)
	if err := http.ListenAndServe(listenAddr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "地图服务监听失败：%v\n", err)
		os.Exit(1)
	}
}

func restoreCheckpoint(client *cluster.Client, name string, service *cloud.MapService) {
	var cp protocol.MapCheckpoint
	ok, err := client.LoadJSON(context.Background(), name, "checkpoint.json", &cp)
	if err == nil && ok {
		service.RestoreCheckpoint(cp)
	}
}

func checkpointLoop(ctx context.Context, client *cluster.Client, name string, service *cloud.MapService, elector *cluster.LeaderElector) {
	labels := map[string]string{"app.kubernetes.io/part-of": "lab4", "lab4/state": "map"}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if elector.IsLeader() {
				_ = client.SaveJSON(ctx, name, "checkpoint.json", labels, service.Checkpoint())
				continue
			}
			restoreCheckpoint(client, name, service)
		case <-ctx.Done():
			return
		}
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
