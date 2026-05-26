package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"battleworld/redisx"
)

type RedisLeaderElector struct {
	client   *redisx.Client
	key      string
	podName  string
	podIP    string
	port     string
	ttl      time.Duration
	eligible atomic.Value
	isLeader atomic.Bool
	leaderIP atomic.Value
	leaderID atomic.Value
}

type redisLeaderInfo struct {
	ID        string    `json:"id"`
	IP        string    `json:"ip"`
	Port      string    `json:"port"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewRedisLeaderElector(client *redisx.Client, prefix, component, listenAddr string) *RedisLeaderElector {
	prefix = firstNonEmpty(prefix, "lab4")
	ttl := 6 * time.Second
	if raw := os.Getenv("LAB4_LEADER_TTL_SECONDS"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 2 {
			ttl = time.Duration(seconds) * time.Second
		}
	}
	return &RedisLeaderElector{
		client:  client,
		key:     sanitizeName(prefix + ":leader:" + component),
		podName: firstNonEmpty(os.Getenv("POD_NAME"), hostname()),
		podIP:   firstNonEmpty(os.Getenv("POD_IP"), "127.0.0.1"),
		port:    portOf(listenAddr),
		ttl:     ttl,
	}
}

func (e *RedisLeaderElector) Start(ctx context.Context) {
	e.refresh(ctx)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				e.refresh(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (e *RedisLeaderElector) IsLeader() bool {
	return e.isLeader.Load()
}

func (e *RedisLeaderElector) SetEligible(fn func() bool) {
	e.eligible.Store(fn)
}

func (e *RedisLeaderElector) LeaderIP() string {
	if value, ok := e.leaderIP.Load().(string); ok {
		return value
	}
	return ""
}

func (e *RedisLeaderElector) LeaderID() string {
	if value, ok := e.leaderID.Load().(string); ok {
		return value
	}
	return ""
}

func (e *RedisLeaderElector) ProxyOrServe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || e.IsLeader() {
			next.ServeHTTP(w, r)
			return
		}
		leaderIP := e.LeaderIP()
		if leaderIP == "" || leaderIP == e.podIP {
			http.Error(w, "leader is not available", http.StatusServiceUnavailable)
			return
		}
		target, err := url.Parse(fmt.Sprintf("http://%s", net.JoinHostPort(leaderIP, e.port)))
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		proxy.ServeHTTP(w, r)
	})
}

func (e *RedisLeaderElector) refresh(ctx context.Context) {
	now := time.Now()
	mine := redisLeaderInfo{ID: e.podName, IP: e.podIP, Port: e.port, ExpiresAt: now.Add(e.ttl)}
	mineJSON, _ := json.Marshal(mine)
	eligible := true
	if value := e.eligible.Load(); value != nil {
		if fn, ok := value.(func() bool); ok && !fn() {
			eligible = false
		}
	}

	raw, exists, err := e.client.Get(ctx, e.key)
	if err != nil {
		e.isLeader.Store(false)
		return
	}
	if !eligible {
		var current redisLeaderInfo
		if exists && json.Unmarshal([]byte(raw), &current) == nil && current.ID == e.podName {
			_ = e.client.Del(ctx, e.key)
		}
		e.isLeader.Store(false)
		return
	}
	if !exists {
		if ok, err := e.client.SetNXEX(ctx, e.key, string(mineJSON), e.ttl); err != nil || !ok {
			e.isLeader.Store(false)
			return
		}
		e.storeLeader(mine, true)
		return
	}

	var current redisLeaderInfo
	if err := json.Unmarshal([]byte(raw), &current); err != nil || current.ID == "" || now.After(current.ExpiresAt) {
		if ok, err := e.client.SetNXEX(ctx, e.key, string(mineJSON), e.ttl); err == nil && ok {
			e.storeLeader(mine, true)
			return
		}
		e.isLeader.Store(false)
		return
	}
	if current.ID == e.podName {
		if err := e.client.SetEX(ctx, e.key, string(mineJSON), e.ttl); err != nil {
			e.isLeader.Store(false)
			return
		}
		e.storeLeader(mine, true)
		return
	}
	e.storeLeader(current, false)
}

func (e *RedisLeaderElector) storeLeader(info redisLeaderInfo, mine bool) {
	e.leaderID.Store(info.ID)
	e.leaderIP.Store(info.IP)
	e.isLeader.Store(mine)
}

func sanitizeRedisKeyPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "-")
	return value
}
