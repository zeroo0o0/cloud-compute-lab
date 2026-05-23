package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"battleworld/cloudapi"
	"battleworld/cluster"
	"battleworld/protocol"
)

func main() {
	gatewayAddr := defaultEnv("LAB4_GATEWAY_ADDR", "0.0.0.0:9310")
	healthAddr := defaultEnv("LAB4_GATEWAY_HEALTH_ADDR", "0.0.0.0:9311")
	drainTimeout := durationEnv("LAB4_GATEWAY_DRAIN_TIMEOUT", 300*time.Second)
	preStopDelay := durationEnv("LAB4_GATEWAY_PRESTOP_DELAY", 5*time.Second)
	coordURL := cloudapi.NormalizeBaseURL(requiredEnv("LAB4_COORDINATOR_URL"))

	ln, err := net.Listen("tcp", gatewayAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "网关监听失败：%v\n", err)
		os.Exit(1)
	}
	defer ln.Close()

	var draining atomic.Bool
	var activeConn atomic.Int64
	var drainOnce sync.Once
	beginDrain := func(reason string) {
		drainOnce.Do(func() {
			draining.Store(true)
			_ = ln.Close()
			fmt.Printf("网关进入回收保护：reason=%s active=%d timeout=%s\n", reason, activeConn.Load(), drainTimeout)
		})
	}
	healthServer := startGatewayHealthServer(healthAddr, &draining, &activeConn, beginDrain, preStopDelay)
	defer healthServer.Close()

	ctx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	if err := cluster.StartPodDeletionCostReporter(ctx, defaultEnv("LAB4_NAMESPACE", ""), func() int {
		return int(activeConn.Load())
	}, draining.Load); err != nil && os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		fmt.Fprintf(os.Stderr, "启动 Pod 回收保护上报失败：%v\n", err)
	}

	fmt.Printf("云网关已启动：listen=%s health=%s coordinator=%s\n", gatewayAddr, healthAddr, coordURL)
	var wg sync.WaitGroup
	go func() {
		<-ctx.Done()
		beginDrain("signal")
	}()
	for {
		raw, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || draining.Load() {
				waitForGatewayDrain(&activeConn, &wg, drainTimeout)
				return
			}
			fmt.Fprintf(os.Stderr, "接入连接失败：%v\n", err)
			continue
		}
		if draining.Load() {
			_ = raw.Close()
			continue
		}
		wg.Add(1)
		activeConn.Add(1)
		go func() {
			defer activeConn.Add(-1)
			defer wg.Done()
			handleClient(raw, coordURL, &draining)
		}()
	}
}

func handleClient(raw net.Conn, coordURL string, draining *atomic.Bool) {
	conn := protocol.NewConn(raw)
	defer conn.Close()

	authMsg, err := conn.Receive()
	if err != nil {
		return
	}
	client := cloudapi.NewHTTPClient()

	if authMsg.Type == protocol.TypeAdmin {
		resp, err := callCoordinator(client, coordURL, cloudapi.CoordinatorRequest{
			Action:      cloudapi.CoordinatorActionAdmin,
			AdminAction: authMsg.Action,
			NodeID:      authMsg.NodeID,
		})
		if err != nil {
			_ = conn.Send(protocol.Message{Type: protocol.TypeError, Error: err.Error()})
			return
		}
		_ = conn.Send(protocol.Message{Type: protocol.TypeAdmin, OK: true, Text: resp.Text})
		return
	}
	if draining.Load() {
		_ = conn.Send(protocol.Message{Type: protocol.TypeError, Error: "网关正在回收，请重新连接"})
		return
	}

	resp, err := callCoordinator(client, coordURL, cloudapi.CoordinatorRequest{
		Action:   authMsg.Type,
		Username: authMsg.Username,
		Password: authMsg.Password,
		Confirm:  authMsg.Confirm,
	})
	if err != nil {
		_ = conn.Send(protocol.Message{Type: protocol.TypeError, Error: err.Error()})
		return
	}
	username := authMsg.Username
	if err := conn.Send(protocol.Message{Type: protocol.TypeAuth, OK: true, State: resp.State}); err != nil {
		_, _ = callCoordinator(client, coordURL, cloudapi.CoordinatorRequest{Action: cloudapi.CoordinatorActionLogout, Username: username})
		return
	}

	var once sync.Once
	done := make(chan struct{})
	stop := func() {
		once.Do(func() {
			close(done)
			_, _ = callCoordinator(client, coordURL, cloudapi.CoordinatorRequest{Action: cloudapi.CoordinatorActionLogout, Username: username})
		})
	}
	defer stop()

	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				resp, err := callCoordinator(client, coordURL, cloudapi.CoordinatorRequest{
					Action:   cloudapi.CoordinatorActionSnapshot,
					Username: username,
				})
				if err != nil {
					_ = conn.Send(protocol.Message{Type: protocol.TypeError, Error: err.Error()})
					stop()
					return
				}
				if err := conn.Send(protocol.Message{Type: protocol.TypeState, State: resp.State}); err != nil {
					stop()
					return
				}
			case <-done:
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		default:
		}

		msg, err := conn.Receive()
		if err != nil {
			return
		}

		if msg.Type == protocol.TypeLogout {
			return
		}

		req := cloudapi.CoordinatorRequest{
			Action:   msg.Type,
			Username: username,
			Dir:      msg.Dir,
			MapID:    msg.MapID,
			Item:     msg.Item,
			Target:   msg.Target,
			Amount:   msg.Amount,
		}
		resp, err := callCoordinator(client, coordURL, req)
		if err != nil {
			if sendErr := conn.Send(protocol.Message{Type: protocol.TypeError, Error: err.Error()}); sendErr != nil {
				return
			}
			continue
		}
		if resp.State != nil {
			if err := conn.Send(protocol.Message{Type: protocol.TypeState, State: resp.State}); err != nil {
				return
			}
		}
	}
}

func callCoordinator(client *http.Client, coordURL string, req cloudapi.CoordinatorRequest) (cloudapi.CoordinatorResponse, error) {
	var resp cloudapi.CoordinatorResponse
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		resp = cloudapi.CoordinatorResponse{}
		if err := cloudapi.PostJSON(client, coordURL+"/v1/coordinator", req, &resp); err != nil {
			lastErr = err
			time.Sleep(time.Duration(120+attempt*180) * time.Millisecond)
			continue
		}
		if !resp.OK {
			return resp, fmt.Errorf("%s", resp.Error)
		}
		return resp, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("协调器请求失败")
	}
	return resp, lastErr
}

func startGatewayHealthServer(addr string, draining *atomic.Bool, active *atomic.Int64, beginDrain func(string), preStopDelay time.Duration) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "ok active=%d draining=%t\n", active.Load(), draining.Load())
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if draining.Load() {
			http.Error(w, "draining", http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprintf(w, "ready active=%d\n", active.Load())
	})
	mux.HandleFunc("/drain", func(w http.ResponseWriter, _ *http.Request) {
		beginDrain("preStop")
		if preStopDelay > 0 {
			time.Sleep(preStopDelay)
		}
		_, _ = fmt.Fprintf(w, "draining active=%d\n", active.Load())
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "网关健康检查服务失败：%v\n", err)
		}
	}()
	return server
}

func waitForGatewayDrain(active *atomic.Int64, wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	if timeout <= 0 {
		<-done
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-done:
			fmt.Println("网关回收完成：所有玩家连接已自然结束")
			return
		case <-ticker.C:
			fmt.Printf("网关回收等待中：active=%d\n", active.Load())
		case <-timer.C:
			fmt.Printf("网关回收超时：仍有 active=%d 个连接，将交给 Kubernetes 结束\n", active.Load())
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
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}
