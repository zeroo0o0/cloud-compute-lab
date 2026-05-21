package cluster

import (
	"context"
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
)

type LeaderElector struct {
	client    *Client
	name      string
	component string
	podName   string
	podIP     string
	port      string
	ttl       time.Duration
	eligible  atomic.Value
	isLeader  atomic.Bool
	leaderIP  atomic.Value
	leaderID  atomic.Value
}

func NewLeaderElector(client *Client, component, listenAddr string) *LeaderElector {
	podName := firstNonEmpty(os.Getenv("POD_NAME"), hostname())
	podIP := firstNonEmpty(os.Getenv("POD_IP"), "127.0.0.1")
	prefix := firstNonEmpty(os.Getenv("LAB4_LEASE_PREFIX"), "lab4-leader")
	ttl := 10 * time.Second
	if raw := os.Getenv("LAB4_LEADER_TTL_SECONDS"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 2 {
			ttl = time.Duration(seconds) * time.Second
		}
	}
	return &LeaderElector{
		client:    client,
		name:      sanitizeName(prefix + "-" + component),
		component: component,
		podName:   podName,
		podIP:     podIP,
		port:      portOf(listenAddr),
		ttl:       ttl,
	}
}

func (e *LeaderElector) Start(ctx context.Context) {
	e.refresh(ctx)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
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

func (e *LeaderElector) IsLeader() bool {
	return e.isLeader.Load()
}

func (e *LeaderElector) SetEligible(fn func() bool) {
	e.eligible.Store(fn)
}

func (e *LeaderElector) LeaderIP() string {
	if value, ok := e.leaderIP.Load().(string); ok {
		return value
	}
	return ""
}

func (e *LeaderElector) LeaderID() string {
	if value, ok := e.leaderID.Load().(string); ok {
		return value
	}
	return ""
}

func (e *LeaderElector) ProxyOrServe(next http.Handler) http.Handler {
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

func (e *LeaderElector) refresh(ctx context.Context) {
	now := time.Now()
	expiresAt := now.Add(e.ttl).UTC().Format(time.RFC3339Nano)
	labels := map[string]string{"app.kubernetes.io/part-of": "lab4", "lab4/component": e.component}
	eligible := true
	if value := e.eligible.Load(); value != nil {
		if fn, ok := value.(func() bool); ok && !fn() {
			eligible = false
		}
	}
	_ = e.client.UpdateConfigMapData(ctx, e.name, labels, func(data map[string]string) error {
		currentLeader := data["leaderID"]
		currentExpiry, _ := time.Parse(time.RFC3339Nano, data["expiresAt"])
		if !eligible {
			if currentLeader == e.podName {
				data["expiresAt"] = now.Add(-time.Second).UTC().Format(time.RFC3339Nano)
			}
			return nil
		}
		if currentLeader == "" || currentLeader == e.podName || now.After(currentExpiry) {
			data["leaderID"] = e.podName
			data["leaderIP"] = e.podIP
			data["expiresAt"] = expiresAt
			data["component"] = e.component
		}
		return nil
	})
	cm, exists, err := e.client.GetConfigMap(ctx, e.name)
	if err != nil || !exists {
		e.isLeader.Store(false)
		return
	}
	leaderID := cm.Data["leaderID"]
	leaderIP := cm.Data["leaderIP"]
	expiry, _ := time.Parse(time.RFC3339Nano, cm.Data["expiresAt"])
	isCurrent := eligible && leaderID == e.podName && time.Now().Before(expiry)
	e.leaderID.Store(leaderID)
	e.leaderIP.Store(leaderIP)
	e.isLeader.Store(isCurrent)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func hostname() string {
	name, _ := os.Hostname()
	if name == "" {
		return "unknown"
	}
	return name
}

func portOf(addr string) string {
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "" {
		return port
	}
	if strings.HasPrefix(addr, ":") {
		return strings.TrimPrefix(addr, ":")
	}
	return "80"
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
