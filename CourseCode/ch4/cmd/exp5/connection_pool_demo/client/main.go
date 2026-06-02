package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type pooledConn struct {
	conn      net.Conn
	reader    *bufio.Reader
	createdAt time.Time
}

type poolConfig struct {
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
}

type connectionPool struct {
	addr string
	cfg  poolConfig
	idle chan *pooledConn

	mu       sync.Mutex
	openConn int

	dials         atomic.Int64
	reused        atomic.Int64
	expiredClosed atomic.Int64
	idleClosed    atomic.Int64
}

type scenarioResult struct {
	name           string
	requests       int
	dials          int64
	reused         int64
	expiredClosed  int64
	idleClosed     int64
	duration       time.Duration
	maxOpenConns   int
	maxIdleConns   int
	connMaxLife    time.Duration
	successReplies int64
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9205", "服务端地址")
	requests := flag.Int("requests", 160, "请求总数")
	concurrency := flag.Int("concurrency", 16, "并发请求数")
	maxOpen := flag.Int("max-open", 16, "连接池最大打开连接数")
	maxIdle := flag.Int("max-idle", 8, "连接池最大空闲连接数")
	lifetimeMS := flag.Int("lifetime-ms", 120, "连接最大生命周期（毫秒）")
	flag.Parse()

	if *requests < 1 || *concurrency < 1 || *maxOpen < 1 || *maxIdle < 1 {
		fmt.Println("requests、concurrency、max-open、max-idle 都必须大于 0")
		return
	}

	cfg := poolConfig{
		maxOpenConns:    *maxOpen,
		maxIdleConns:    *maxIdle,
		connMaxLifetime: time.Duration(*lifetimeMS) * time.Millisecond,
	}

	fmt.Println("=== 实验五：网络连接池客户端 ===")
	fmt.Println("场景：连接到独立 TCP server，分别运行短连接和连接池两组请求。")
	fmt.Println("初始化配置（贴近 PPT 的连接池三件套）：")
	fmt.Printf("  maxOpenConns    = %d\n", cfg.maxOpenConns)
	fmt.Printf("  maxIdleConns    = %d\n", cfg.maxIdleConns)
	fmt.Printf("  connMaxLifetime = %s\n", cfg.connMaxLifetime)
	fmt.Printf("  requests=%d, concurrency=%d, server=%s\n\n", *requests, *concurrency, *addr)

	shortConn := runShortConnection(*addr, *requests, *concurrency)
	pooledConn := runConnectionPool(*addr, *requests, *concurrency, cfg)

	fmt.Println("结果对比：")
	printResult(shortConn)
	printResult(pooledConn)

	fmt.Println()
	fmt.Printf("短连接 / 连接池：耗时约 %.1f 倍，建连次数从 %d 降到 %d。\n",
		float64(shortConn.duration)/float64(pooledConn.duration),
		shortConn.dials,
		pooledConn.dials,
	)
	fmt.Println("[结论] 连接池的核心不是“永不关连接”，而是用 maxOpen / maxIdle / lifetime 控制连接的创建、复用和淘汰。")
}

func requestOnce(conn net.Conn, reader *bufio.Reader, requestID int) error {
	if _, err := fmt.Fprintf(conn, "attack:%d\n", requestID); err != nil {
		return err
	}

	reply, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(reply, "ok:attack:") {
		return fmt.Errorf("unexpected reply: %q", strings.TrimSpace(reply))
	}
	return nil
}

func runShortConnection(addr string, requests, concurrency int) scenarioResult {
	start := time.Now()
	jobs := make(chan int, concurrency)
	var wg sync.WaitGroup
	var dials int64
	var responses int64

	/*
		================ 【学生重点 实验五：短连接反例】 ================
		请只看 worker 里面这组代码：
		1. net.Dial：每个请求都新建连接。
		2. requestOnce(...)
		3. conn.Close()：请求结束立刻断开。

		所以请求越多，重复建连和断连的成本就越多。
		============================================================
	*/
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for requestID := range jobs {
				conn, err := net.Dial("tcp", addr)
				if err != nil {
					fmt.Printf("[短连接] dial failed: %v\n", err)
					continue
				}
				atomic.AddInt64(&dials, 1)

				reader := bufio.NewReader(conn)
				if err := requestOnce(conn, reader, requestID); err != nil {
					fmt.Printf("[短连接] request failed: %v\n", err)
				} else {
					atomic.AddInt64(&responses, 1)
				}
				_ = conn.Close()
			}
		}()
	}

	for requestID := 0; requestID < requests; requestID++ {
		jobs <- requestID
	}
	close(jobs)
	wg.Wait()

	return scenarioResult{
		name:           "短连接：每个请求 Dial + Close",
		requests:       requests,
		dials:          dials,
		duration:       time.Since(start),
		successReplies: responses,
	}
}

func initConnectionPool(addr string, cfg poolConfig) *connectionPool {
	if cfg.maxIdleConns > cfg.maxOpenConns {
		cfg.maxIdleConns = cfg.maxOpenConns
	}

	/*
		================ 【学生重点 实验五：连接池初始化】 ================
		这里故意写成和 PPT 很接近的三项配置：
		1. maxOpenConns：最多同时打开多少条连接。
		2. maxIdleConns：最多保留多少条空闲连接。
		3. connMaxLifetime：连接活多久后就视为过期，需要重建。

		这就是“短连接 -> 长连接池”里最常见的初始化配置。
		==============================================================
	*/
	return &connectionPool{
		addr: addr,
		cfg:  cfg,
		idle: make(chan *pooledConn, cfg.maxIdleConns),
	}
}

func newPooledConn(addr string) (*pooledConn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &pooledConn{
		conn:      conn,
		reader:    bufio.NewReader(conn),
		createdAt: time.Now(),
	}, nil
}

func (p *connectionPool) isExpired(pc *pooledConn) bool {
	if p.cfg.connMaxLifetime <= 0 {
		return false
	}
	return time.Since(pc.createdAt) >= p.cfg.connMaxLifetime
}

func (p *connectionPool) closeConn(pc *pooledConn) {
	_ = pc.conn.Close()
	p.mu.Lock()
	p.openConn--
	p.mu.Unlock()
}

func (p *connectionPool) dialNew() (*pooledConn, error) {
	pc, err := newPooledConn(p.addr)
	if err != nil {
		p.mu.Lock()
		p.openConn--
		p.mu.Unlock()
		return nil, err
	}
	p.dials.Add(1)
	return pc, nil
}

func (p *connectionPool) get() (*pooledConn, error) {
	for {
		select {
		case pc := <-p.idle:
			if p.isExpired(pc) {
				p.expiredClosed.Add(1)
				p.closeConn(pc)
				continue
			}
			p.reused.Add(1)
			return pc, nil
		default:
		}

		p.mu.Lock()
		if p.openConn < p.cfg.maxOpenConns {
			p.openConn++
			p.mu.Unlock()
			return p.dialNew()
		}
		p.mu.Unlock()

		pc := <-p.idle
		if p.isExpired(pc) {
			p.expiredClosed.Add(1)
			p.closeConn(pc)
			continue
		}
		p.reused.Add(1)
		return pc, nil
	}
}

func (p *connectionPool) put(pc *pooledConn) {
	if p.isExpired(pc) {
		p.expiredClosed.Add(1)
		p.closeConn(pc)
		return
	}

	select {
	case p.idle <- pc:
	default:
		p.idleClosed.Add(1)
		p.closeConn(pc)
	}
}

func (p *connectionPool) discard(pc *pooledConn) {
	p.closeConn(pc)
}

func (p *connectionPool) close() {
	close(p.idle)
	for pc := range p.idle {
		_ = pc.conn.Close()
	}
}

func runConnectionPool(addr string, requests, concurrency int, cfg poolConfig) scenarioResult {
	start := time.Now()
	pool := initConnectionPool(addr, cfg)
	defer pool.close()

	jobs := make(chan int, concurrency)
	var wg sync.WaitGroup
	var responses int64

	/*
		================ 【学生重点 实验五：借还长连接】 ================
		请只看下面两步：
		1. pc, err := pool.get()：借连接。
		2. pool.put(pc)：归还连接。

		业务请求很多，但真正反复创建的新连接数量会被限制在 maxOpenConns 附近。
		空闲连接数则受 maxIdleConns 控制。
		============================================================
	*/
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for requestID := range jobs {
				pc, err := pool.get()
				if err != nil {
					fmt.Printf("[连接池] get failed: %v\n", err)
					continue
				}

				if err := requestOnce(pc.conn, pc.reader, requestID); err != nil {
					fmt.Printf("[连接池] request failed: %v\n", err)
					pool.discard(pc)
					continue
				}

				atomic.AddInt64(&responses, 1)
				pool.put(pc)
			}
		}()
	}

	for requestID := 0; requestID < requests; requestID++ {
		jobs <- requestID
	}
	close(jobs)
	wg.Wait()

	return scenarioResult{
		name:           "连接池：按配置复用长连接",
		requests:       requests,
		dials:          pool.dials.Load(),
		reused:         pool.reused.Load(),
		expiredClosed:  pool.expiredClosed.Load(),
		idleClosed:     pool.idleClosed.Load(),
		duration:       time.Since(start),
		maxOpenConns:   cfg.maxOpenConns,
		maxIdleConns:   cfg.maxIdleConns,
		connMaxLife:    cfg.connMaxLifetime,
		successReplies: responses,
	}
}

func printResult(result scenarioResult) {
	if result.maxOpenConns == 0 && result.maxIdleConns == 0 {
		fmt.Printf("%-30s requests=%4d  成功响应=%4d  建连次数=%4d  耗时=%9s\n",
			result.name,
			result.requests,
			result.successReplies,
			result.dials,
			result.duration.Round(time.Millisecond),
		)
		return
	}

	fmt.Printf("%-30s requests=%4d  成功响应=%4d  建连=%3d  复用=%4d  过期关闭=%3d  空闲淘汰=%3d  耗时=%9s\n",
		result.name,
		result.requests,
		result.successReplies,
		result.dials,
		result.reused,
		result.expiredClosed,
		result.idleClosed,
		result.duration.Round(time.Millisecond),
	)
}
