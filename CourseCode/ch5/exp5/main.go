package main

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	serverCount  = 100
	fanout       = 2
	rounds       = 12
	summaryDelay = 50 * time.Millisecond
)

// BossEvent 是被传播的“世界 Boss 已刷新”消息。
// SourceID 用来记录这条消息从哪台服务器传来，便于观察传播路径。
type BossEvent struct {
	SourceID int
	Round    int
}

// Server 表示一台轻量级游戏服务器。
// 每台服务器只知道自己是否已经发现 Boss，并通过 inbox 接收其他服务器的异步通知。
type Server struct {
	ID           int
	inbox        chan BossEvent
	roundCh      chan int
	knownAtRound atomic.Int32
	hasBoss      atomic.Bool
}

func main() {
	// 初始化随机种子，让每次运行时服务器挑选的 gossip 邻居都不完全一样。
	rand.Seed(time.Now().UnixNano())
	// ctx/cancel 用来统一通知 100 个服务器 goroutine 结束。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 100 台模拟服务器；每台服务器都有一个 inbox，用来接收其他服务器的异步通知。
	servers := make([]*Server, 0, serverCount)
	for i := 0; i < serverCount; i++ {
		s := &Server{
			ID:      i,
			inbox:   make(chan BossEvent, 16),
			roundCh: make(chan int),
		}
		s.knownAtRound.Store(-1)
		servers = append(servers, s)
	}

	// 每台服务器各启动一个 goroutine，模拟真实分布式环境里多台服务器同时运行。
	var wg sync.WaitGroup
	for _, s := range servers {
		wg.Add(1)
		go func(server *Server) {
			defer wg.Done()
			server.run(ctx, servers)
		}(s)
	}

	fmt.Printf("启动 %d 台服务器，fanout=%d", serverCount, fanout)
	fmt.Println("向 Server-000 注入 HasBoss=true，观察消息如何去中心化扩散。")

	// 从 1 台服务器开始注入 Boss 状态，后续传播完全依赖各服务器自己 gossip。
	servers[0].discover(0, -1)

	reader := bufio.NewReader(os.Stdin)
	waitForEnter(reader, "按回车开始第 01 轮 gossip...")

	for round := 1; round <= rounds; round++ {
		for _, s := range servers {
			s.roundCh <- round
		}
		// 稍等一下，让这一轮服务器之间的 gossip 日志先打印出来，再输出汇总。
		time.Sleep(summaryDelay)

		known := countKnown(servers)
		fmt.Printf("[第 %02d 轮汇总] 已发现世界 Boss 的服务器：%3d/%d\n", round, known, serverCount)
		if known == serverCount {
			fmt.Printf("全部服务器已同步 Boss 状态，总轮数=%d，接近 O(log N) 的传播效果。\n", round)
			break
		}
		if round < rounds {
			waitForEnter(reader, fmt.Sprintf("按回车继续第 %02d 轮 gossip...", round+1))
		}
	}

	// 实验结束后通知所有服务器退出，并等待它们收尾。
	cancel()
	wg.Wait()

	// 打印每一轮首次发现 Boss 的服务器数量，方便观察指数级扩散趋势。
	printDistribution(servers)
}

// run 持续处理外部通知；只要自己已经知道 Boss 存在，就会定期随机挑选 fanout 个邻居传播。
func (s *Server) run(ctx context.Context, all []*Server) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-s.inbox:
			s.discover(event.Round, event.SourceID)
		case currentRound := <-s.roundCh:
			// 本轮刚发现 Boss 的服务器，从下一轮开始继续传播，避免一轮内连锁转发导致轮次含义变乱。
			if s.hasBoss.Load() && int(s.knownAtRound.Load()) < currentRound {
				s.gossip(all, currentRound)
			}
		}
	}
}

func waitForEnter(reader *bufio.Reader, prompt string) {
	fmt.Print(prompt)
	_, _ = reader.ReadString('\n')
}

// discover 把服务器从“未知”切换到“已发现 Boss”。
// CompareAndSwap 保证每台服务器只会打印一次首次发现日志。
func (s *Server) discover(round int, sourceID int) {
	if !s.hasBoss.CompareAndSwap(false, true) {
		return
	}
	s.knownAtRound.Store(int32(round))

	if sourceID < 0 {
		fmt.Printf("[第 %02d 轮] Server-%03d 已发现世界 Boss：本机注入 HasBoss=true\n", round, s.ID)
		return
	}
	fmt.Printf("[第 %02d 轮] Server-%03d 已发现世界 Boss：来自 Server-%03d 的 gossip\n", round, s.ID, sourceID)
}

// gossipTo 使用非阻塞 channel 发送，模拟“异步通知”。
// 如果目标 inbox 已满，本轮直接跳过，表示 gossip 不会因为单个慢节点拖垮发送方。
func (s *Server) gossipTo(target *Server, round int) {
	select {
	case target.inbox <- BossEvent{SourceID: s.ID, Round: round}:
	default:
		fmt.Printf("[第 %02d 轮] Server-%03d -> Server-%03d 通知被跳过：目标队列繁忙\n", round, s.ID, target.ID)
	}
}

// gossip 随机挑选 fanout 个不同邻居传播消息。
// 这里没有中心调度器，每台已知服务器都独立决定要通知谁。
func (s *Server) gossip(all []*Server, round int) {
	picked := make(map[int]struct{}, fanout)
	for len(picked) < fanout {
		targetID := rand.Intn(len(all))
		if targetID == s.ID {
			continue
		}
		if _, exists := picked[targetID]; exists {
			continue
		}
		picked[targetID] = struct{}{}
		s.gossipTo(all[targetID], round)
	}
}

func countKnown(servers []*Server) int {
	total := 0
	for _, s := range servers {
		if s.hasBoss.Load() {
			total++
		}
	}
	return total
}

func printDistribution(servers []*Server) {
	byRound := make(map[int]int)
	for _, s := range servers {
		round := int(s.knownAtRound.Load())
		byRound[round]++
	}

	fmt.Println("\n首次发现轮次分布：")
	for round := 0; round <= rounds; round++ {
		if n := byRound[round]; n > 0 {
			fmt.Printf("  第 %02d 轮：%d 台服务器\n", round, n)
		}
	}
	if n := byRound[-1]; n > 0 {
		fmt.Printf("  未发现：%d 台服务器\n", n)
	}
}
