package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"battleworld/cloud"
	"battleworld/cluster"
	"battleworld/protocol"
	"battleworld/redisx"
)

type leaderElector interface {
	Start(context.Context)
	IsLeader() bool
	ProxyOrServe(http.Handler) http.Handler
}

type eligibleLeaderElector interface {
	SetEligible(func() bool)
}

func main() {
	mapID := requiredEnv("LAB4_MAP_ID")
	nodeID := defaultEnv("LAB4_NODE_ID", "map-"+mapID)
	listenAddr := defaultEnv("LAB4_MAP_LISTEN_ADDR", ":9400")
	preStopDelay := durationEnv("LAB4_PRESTOP_DELAY", 5*time.Second)
	drainTimeout := durationEnv("LAB4_MAP_DRAIN_TIMEOUT", 180*time.Second)
	ctx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	var draining atomic.Bool
	var drainOnce sync.Once
	saveFinalCheckpoint := func() {}
	activePlayers := func() int { return 0 }
	beginDrain := func(reason string) {
		drainOnce.Do(func() {
			saveFinalCheckpoint()
			draining.Store(true)
			fmt.Printf("地图服务进入回收保护：map=%s reason=%s active=%d timeout=%s\n", mapID, reason, activePlayers(), drainTimeout)
		})
	}

	service, err := cloud.NewMapService(nodeID, mapID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化地图服务失败：%v\n", err)
		os.Exit(1)
	}
	activePlayers = service.ActivePlayers

	var elector leaderElector
	var kubeClient *cluster.Client
	var redisClient *redisx.Client
	checkpointKey := ""
	stateBackend := strings.ToLower(defaultEnv("LAB4_STATE_BACKEND", ""))
	if stateBackend == "redis" || strings.TrimSpace(os.Getenv("LAB4_REDIS_ADDR")) != "" {
		redisClient = redisx.FromEnv("lab4-redis:6379")
		prefix := defaultEnv("LAB4_REDIS_PREFIX", "lab4")
		checkpointKey = prefix + ":checkpoint:" + mapID
		restoreRedisCheckpoint(redisClient, checkpointKey, service)
		elector = cluster.NewRedisLeaderElector(redisClient, prefix, defaultEnv("LAB4_COMPONENT", "map-"+mapID), listenAddr)
		service.SetAfterMutation(func() {
			if elector.IsLeader() {
				_ = saveRedisCheckpoint(redisClient, checkpointKey, service)
			}
		})
		go redisCheckpointLoop(ctx, redisClient, checkpointKey, service, elector)
	} else if strings.EqualFold(defaultEnv("LAB4_KUBE_STATE", "false"), "true") {
		kubeClient, err = cluster.NewInClusterClient(defaultEnv("LAB4_NAMESPACE", ""))
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化 Kubernetes 状态客户端失败：%v\n", err)
			os.Exit(1)
		}
		checkpointName := defaultEnv("LAB4_MAP_CHECKPOINT", "lab4-map-"+mapID+"-checkpoint")
		restoreCheckpoint(kubeClient, checkpointName, service)
		elector = cluster.NewLeaderElector(kubeClient, defaultEnv("LAB4_COMPONENT", "map-"+mapID), listenAddr)
		go checkpointLoop(ctx, kubeClient, checkpointName, service, elector)
		saveFinalCheckpoint = func() {
			if elector != nil && elector.IsLeader() {
				_ = kubeClient.SaveJSON(context.Background(), checkpointName, "checkpoint.json", mapStateLabels(), service.Checkpoint())
			}
		}
	}
	if redisClient != nil && checkpointKey != "" {
		saveFinalCheckpoint = func() {
			if elector != nil && elector.IsLeader() {
				_ = saveRedisCheckpoint(redisClient, checkpointKey, service)
			}
		}
	}

	stop := make(chan struct{})
	if elector != nil {
		if eligible, ok := elector.(eligibleLeaderElector); ok {
			eligible.SetEligible(func() bool { return !draining.Load() })
		}
		elector.Start(ctx)
		service.StartBackgroundWhen(stop, func() bool {
			return !draining.Load() && elector.IsLeader()
		})
	} else {
		service.StartBackground(stop)
	}
	if err := cluster.StartPodDeletionCostReporter(ctx, defaultEnv("LAB4_NAMESPACE", ""), service.ActivePlayers, draining.Load); err != nil && os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		fmt.Fprintf(os.Stderr, "启动 Pod 回收保护上报失败：%v\n", err)
	}

	handler := service.Handler()
	if elector != nil {
		handler = elector.ProxyOrServe(handler)
	}
	handler = cluster.LifecycleHandler(handler, &draining, service.ActivePlayers, beginDrain, preStopDelay)

	fmt.Printf("地图服务已启动：map=%s node=%s listen=%s\n", mapID, nodeID, listenAddr)
	server := &http.Server{Addr: listenAddr, Handler: handler, ReadHeaderTimeout: 3 * time.Second}
	go func() {
		<-ctx.Done()
		beginDrain("signal")
		close(stop)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

func checkpointLoop(ctx context.Context, client *cluster.Client, name string, service *cloud.MapService, elector leaderElector) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if elector.IsLeader() {
				_ = client.SaveJSON(ctx, name, "checkpoint.json", mapStateLabels(), service.Checkpoint())
				continue
			}
			restoreCheckpoint(client, name, service)
		case <-ctx.Done():
			return
		}
	}
}

func mapStateLabels() map[string]string {
	return map[string]string{"app.kubernetes.io/part-of": "lab4", "lab4/state": "map"}
}

func restoreRedisCheckpoint(client *redisx.Client, key string, service *cloud.MapService) {
	var cp protocol.MapCheckpoint
	ok, err := client.GetJSON(context.Background(), key, &cp)
	if err == nil && ok {
		service.RestoreCheckpoint(cp)
	}
}

func saveRedisCheckpoint(client *redisx.Client, key string, service *cloud.MapService) error {
	return client.SetJSON(context.Background(), key, service.Checkpoint())
}

func redisCheckpointLoop(ctx context.Context, client *redisx.Client, key string, service *cloud.MapService, elector leaderElector) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if elector.IsLeader() {
				_ = saveRedisCheckpoint(client, key, service)
				continue
			}
			restoreRedisCheckpoint(client, key, service)
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

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
		return parsed
	}
	return fallback
}
