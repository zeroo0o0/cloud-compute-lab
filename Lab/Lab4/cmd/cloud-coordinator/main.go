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
	"battleworld/redisx"
	"battleworld/storage"
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
	dataRoot := defaultEnv("LAB4_DATA_ROOT", ".")
	listenAddr := defaultEnv("LAB4_COORDINATOR_ADDR", ":9320")
	preStopDelay := durationEnv("LAB4_PRESTOP_DELAY", 5*time.Second)
	drainTimeout := durationEnv("LAB4_COORDINATOR_DRAIN_TIMEOUT", 180*time.Second)
	ctx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	var draining atomic.Bool
	var drainOnce sync.Once
	activePlayers := func() int { return 0 }
	beginDrain := func(reason string) {
		drainOnce.Do(func() {
			draining.Store(true)
			fmt.Printf("协调器进入回收保护：reason=%s active=%d timeout=%s\n", reason, activePlayers(), drainTimeout)
		})
	}

	var store cloud.Store
	var elector leaderElector
	stateBackend := strings.ToLower(defaultEnv("LAB4_STATE_BACKEND", ""))
	if stateBackend == "redis" || strings.TrimSpace(os.Getenv("LAB4_REDIS_ADDR")) != "" {
		redisClient := redisx.FromEnv("lab4-redis:6379")
		prefix := defaultEnv("LAB4_REDIS_PREFIX", "lab4")
		store = storage.NewRedisStore(redisClient, prefix)
		elector = cluster.NewRedisLeaderElector(redisClient, prefix, defaultEnv("LAB4_COMPONENT", "coordinator"), listenAddr)
	} else if strings.EqualFold(defaultEnv("LAB4_KUBE_STATE", "false"), "true") {
		client, err := cluster.NewInClusterClient(defaultEnv("LAB4_NAMESPACE", ""))
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化 Kubernetes 状态客户端失败：%v\n", err)
			os.Exit(1)
		}
		store = storage.NewKubeStore(client, defaultEnv("LAB4_COORDINATOR_STATE", "lab4-coordinator-state"))
		elector = cluster.NewLeaderElector(client, defaultEnv("LAB4_COMPONENT", "coordinator"), listenAddr)
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
	activePlayers = coordinator.ActivePlayers
	if elector != nil {
		if eligible, ok := elector.(eligibleLeaderElector); ok {
			eligible.SetEligible(func() bool { return !draining.Load() })
		}
		elector.Start(ctx)
		coordinator.SetActivePredicate(elector.IsLeader)
	}
	coordinator.Start()
	if err := cluster.StartPodDeletionCostReporter(ctx, defaultEnv("LAB4_NAMESPACE", ""), coordinator.ActivePlayers, draining.Load); err != nil && os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		fmt.Fprintf(os.Stderr, "启动 Pod 回收保护上报失败：%v\n", err)
	}

	handler := coordinator.Handler()
	if elector != nil {
		handler = elector.ProxyOrServe(handler)
	}
	handler = cluster.LifecycleHandler(handler, &draining, coordinator.ActivePlayers, beginDrain, preStopDelay)

	fmt.Printf("协调器已启动：listen=%s green=%s cave=%s ruins=%s\n", listenAddr, requiredEnv("LAB4_GREEN_URL"), requiredEnv("LAB4_CAVE_URL"), requiredEnv("LAB4_RUINS_URL"))
	server := &http.Server{Addr: listenAddr, Handler: handler, ReadHeaderTimeout: 3 * time.Second}
	go func() {
		<-ctx.Done()
		beginDrain("signal")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
