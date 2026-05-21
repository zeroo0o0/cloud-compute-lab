package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"battleworld/protocol"
)

type counters struct {
	started      atomic.Int64
	active       atomic.Int64
	authOK       atomic.Int64
	dialErrors   atomic.Int64
	authErrors   atomic.Int64
	sendOK       atomic.Int64
	sendErrors   atomic.Int64
	recvOK       atomic.Int64
	serverErrors atomic.Int64
	readErrors   atomic.Int64
	clientDone   atomic.Int64
}

type config struct {
	addr         string
	clients      int
	spawnRate    int
	opsPerClient float64
	duration     time.Duration
	password     string
	prefix       string
	actions      []string
}

func main() {
	var cfg config
	actionList := ""
	flag.StringVar(&cfg.addr, "addr", "120.79.8.174:30910", "gateway address")
	flag.IntVar(&cfg.clients, "clients", 300, "number of concurrent game clients")
	flag.IntVar(&cfg.spawnRate, "spawn-rate", 30, "new clients per second")
	flag.Float64Var(&cfg.opsPerClient, "ops-per-client", 5, "operations per connected client per second")
	flag.DurationVar(&cfg.duration, "duration", 8*time.Minute, "load duration")
	flag.StringVar(&cfg.password, "password", "lab4-loadtest", "password used by generated users")
	flag.StringVar(&cfg.prefix, "prefix", "", "username prefix; defaults to hpa-<timestamp>")
	flag.StringVar(&actionList, "actions", "move,move,move,attack,boss,heal,switch", "comma-separated action mix: move,attack,boss,heal,switch,shop")
	flag.Parse()

	if cfg.clients <= 0 || cfg.spawnRate <= 0 || cfg.duration <= 0 {
		fmt.Fprintln(os.Stderr, "clients、spawn-rate、duration 都必须大于 0")
		os.Exit(2)
	}
	if cfg.opsPerClient < 0 {
		fmt.Fprintln(os.Stderr, "ops-per-client 不能小于 0")
		os.Exit(2)
	}
	if cfg.prefix == "" {
		cfg.prefix = fmt.Sprintf("hpa-%d", time.Now().Unix())
	}
	cfg.actions = parseActions(actionList)
	if len(cfg.actions) == 0 && cfg.opsPerClient > 0 {
		fmt.Fprintln(os.Stderr, "actions 为空时 ops-per-client 必须为 0")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.duration)
	defer cancel()

	stats := &counters{}
	start := time.Now()
	fmt.Printf("gateway load test started: addr=%s clients=%d spawn_rate=%d/s ops_per_client=%.2f duration=%s prefix=%s\n",
		cfg.addr, cfg.clients, cfg.spawnRate, cfg.opsPerClient, cfg.duration, cfg.prefix)
	fmt.Println("tip: HPA 通常需要 1-2 个 metrics-server 采样周期才会扩容，压测时长建议至少 5 分钟。")

	var wg sync.WaitGroup
	go printStats(ctx, start, stats)

	spawnInterval := time.Second / time.Duration(cfg.spawnRate)
	if spawnInterval <= 0 {
		spawnInterval = time.Nanosecond
	}
	spawnTicker := time.NewTicker(spawnInterval)
	defer spawnTicker.Stop()

spawnLoop:
	for i := 0; i < cfg.clients; i++ {
		select {
		case <-ctx.Done():
			break spawnLoop
		case <-spawnTicker.C:
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				runClient(ctx, cfg, id, stats)
			}(i)
		}
	}

	<-ctx.Done()
	wg.Wait()
	printSnapshot("final", time.Since(start), stats, &lastSnapshot{})
}

func parseActions(raw string) []string {
	var actions []string
	for _, part := range strings.Split(raw, ",") {
		action := strings.ToLower(strings.TrimSpace(part))
		switch action {
		case "", "none":
			continue
		case "move", "attack", "boss", "heal", "switch", "shop":
			actions = append(actions, action)
		default:
			fmt.Fprintf(os.Stderr, "忽略未知 action %q\n", action)
		}
	}
	return actions
}

func runClient(ctx context.Context, cfg config, id int, stats *counters) {
	username := fmt.Sprintf("%s-%05d", cfg.prefix, id)
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(id)*7919))
	stats.started.Add(1)

	raw, err := net.DialTimeout("tcp", cfg.addr, 5*time.Second)
	if err != nil {
		stats.dialErrors.Add(1)
		return
	}
	defer raw.Close()

	conn := protocol.NewConn(raw)
	_ = raw.SetDeadline(time.Now().Add(8 * time.Second))
	if err := conn.Send(protocol.Message{
		Type:     protocol.TypeQuickEnter,
		Username: username,
		Password: cfg.password,
		Confirm:  cfg.password,
	}); err != nil {
		stats.authErrors.Add(1)
		return
	}
	authResp, err := conn.Receive()
	if err != nil || !authResp.OK || authResp.Type == protocol.TypeError {
		stats.authErrors.Add(1)
		return
	}
	_ = raw.SetDeadline(time.Time{})

	stats.authOK.Add(1)
	stats.active.Add(1)
	defer func() {
		_ = conn.Send(protocol.Message{Type: protocol.TypeLogout})
		stats.active.Add(-1)
		stats.clientDone.Add(1)
	}()

	readDone := make(chan struct{})
	go readLoop(ctx, conn, stats, readDone)

	interval := opInterval(cfg.opsPerClient)
	if interval == 0 {
		<-ctx.Done()
		return
	}
	timer := time.NewTimer(jitter(interval, rng))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-readDone:
			return
		case <-timer.C:
			msg := nextMessage(cfg.actions, rng)
			if err := conn.Send(msg); err != nil {
				stats.sendErrors.Add(1)
				return
			}
			stats.sendOK.Add(1)
			timer.Reset(jitter(interval, rng))
		}
	}
}

func readLoop(ctx context.Context, conn *protocol.Conn, stats *counters, done chan<- struct{}) {
	defer close(done)
	for {
		msg, err := conn.Receive()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			stats.readErrors.Add(1)
			return
		}
		if msg.Type == protocol.TypeError || !msg.OK && msg.Error != "" {
			stats.serverErrors.Add(1)
			continue
		}
		stats.recvOK.Add(1)
	}
}

func opInterval(opsPerClient float64) time.Duration {
	if opsPerClient <= 0 {
		return 0
	}
	return time.Duration(float64(time.Second) / opsPerClient)
}

func jitter(base time.Duration, rng *rand.Rand) time.Duration {
	if base <= 0 {
		return base
	}
	// Spread clients out so the gateway sees steady pressure instead of synchronized bursts.
	factor := 0.75 + rng.Float64()*0.5
	return time.Duration(float64(base) * factor)
}

func nextMessage(actions []string, rng *rand.Rand) protocol.Message {
	action := actions[rng.Intn(len(actions))]
	switch action {
	case "move":
		return protocol.Message{Type: protocol.TypeMove, Dir: randomDir(rng)}
	case "attack":
		return protocol.Message{Type: protocol.TypeAttack}
	case "boss":
		return protocol.Message{Type: protocol.TypeBossAttack}
	case "heal":
		return protocol.Message{Type: protocol.TypeHeal}
	case "switch":
		return protocol.Message{Type: protocol.TypeSwitchMap, MapID: randomMap(rng)}
	case "shop":
		return protocol.Message{Type: protocol.TypeShop, Item: randomItem(rng)}
	default:
		return protocol.Message{Type: protocol.TypeMove, Dir: randomDir(rng)}
	}
}

func randomDir(rng *rand.Rand) string {
	dirs := []string{protocol.DirUp, protocol.DirDown, protocol.DirLeft, protocol.DirRight}
	return dirs[rng.Intn(len(dirs))]
}

func randomMap(rng *rand.Rand) string {
	maps := []string{"green", "cave", "ruins"}
	return maps[rng.Intn(len(maps))]
}

func randomItem(rng *rand.Rand) string {
	items := []string{"potion", "weapon"}
	return items[rng.Intn(len(items))]
}

type lastSnapshot struct {
	sendOK       int64
	recvOK       int64
	sendErrors   int64
	readErrors   int64
	serverErrors int64
}

func printStats(ctx context.Context, start time.Time, stats *counters) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	last := &lastSnapshot{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			printSnapshot(time.Now().Format("15:04:05"), time.Since(start), stats, last)
		}
	}
}

func printSnapshot(label string, elapsed time.Duration, stats *counters, last *lastSnapshot) {
	sendOK := stats.sendOK.Load()
	recvOK := stats.recvOK.Load()
	sendErrors := stats.sendErrors.Load()
	readErrors := stats.readErrors.Load()
	serverErrors := stats.serverErrors.Load()

	deltaSend := sendOK - last.sendOK
	deltaRecv := recvOK - last.recvOK
	deltaErrors := (sendErrors - last.sendErrors) + (readErrors - last.readErrors) + (serverErrors - last.serverErrors)
	last.sendOK = sendOK
	last.recvOK = recvOK
	last.sendErrors = sendErrors
	last.readErrors = readErrors
	last.serverErrors = serverErrors

	fmt.Printf("[%s +%s] active=%d started=%d auth_ok=%d sent=%d recv=%d dial_err=%d auth_err=%d send_err=%d read_err=%d server_err=%d rate_5s=send:%d/s recv:%d/s err:%d/s\n",
		label,
		elapsed.Truncate(time.Second),
		stats.active.Load(),
		stats.started.Load(),
		stats.authOK.Load(),
		sendOK,
		recvOK,
		stats.dialErrors.Load(),
		stats.authErrors.Load(),
		sendErrors,
		readErrors,
		serverErrors,
		deltaSend/5,
		deltaRecv/5,
		deltaErrors/5,
	)
}
