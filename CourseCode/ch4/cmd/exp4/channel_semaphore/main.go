package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type demoState struct {
	semaphore chan struct{}
	total     int
	workTime  time.Duration

	mu         sync.Mutex
	nextWorker int
	wg         sync.WaitGroup

	dispatched atomic.Int64
	completed  atomic.Int64
}

func newDemoState(total int, limit int, workTime time.Duration) *demoState {
	return &demoState{
		/*
			================ 【学生重点 实验四：Channel 信号量】 ================
			这里的 limit 就是“最多允许几个人同时进副本”。
			Channel 容量是 3，就代表最多只能同时放进 3 个 Worker。

			后面谁想开始工作，必须先往 semaphore 里放一个 struct{}{} 当许可证。
			================================================================
		*/
		semaphore:  make(chan struct{}, limit),
		total:      total,
		workTime:   workTime,
		nextWorker: 1,
	}
}

func (d *demoState) dispatch(count int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := 0; i < count && d.nextWorker <= d.total; i++ {
		id := d.nextWorker
		d.nextWorker++
		d.dispatched.Add(1)
		d.wg.Add(1)

		go func(id int) {
			defer d.wg.Done()

			fmt.Printf("[Worker %d] 已入场，尝试获取许可...\n", id)
			/*
				================ 【学生重点 实验四：获取与释放许可】 ================
				请只看下面两行对 semaphore 的操作：
				1. d.semaphore <- struct{}{}：获取许可。Channel 满了就会阻塞。
				2. <-d.semaphore：释放许可。释放后，排队的 Worker 才能继续。

				所以即使 wave 一次放进很多 Worker，真正同时工作的也不会超过 cap(d.semaphore)。
				==================================================================
			*/
			d.semaphore <- struct{}{}
			fmt.Printf("[Worker %d] 获取许可，当前占用=%d/%d\n", id, len(d.semaphore), cap(d.semaphore))
			time.Sleep(d.workTime)
			fmt.Printf("[Worker %d] 工作完成，释放许可\n", id)
			<-d.semaphore
			d.completed.Add(1)
		}(id)
	}
}

func (d *demoState) waitDispatched() {
	d.wg.Wait()
}

func (d *demoState) printStatus() {
	d.mu.Lock()
	next := d.nextWorker
	d.mu.Unlock()

	fmt.Println("------ 当前状态（信号量） ------")
	fmt.Printf("已派发 Worker: %d / %d\n", d.dispatched.Load(), d.total)
	fmt.Printf("已完成 Worker: %d / %d\n", d.completed.Load(), d.total)
	fmt.Printf("当前许可证占用: %d / %d\n", len(d.semaphore), cap(d.semaphore))
	fmt.Printf("下一位待派发 Worker: %d\n", next)
	fmt.Println("------------------------------")
}

func printHelp() {
	fmt.Println("可用命令:")
	fmt.Println("  status     查看当前并发占用与派发进度")
	fmt.Println("  wave N     让下一批 N 个 Worker 入场，例如 wave 6")
	fmt.Println("  wait       等待当前已派发 Worker 全部结束")
	fmt.Println("  quit       退出演示")
}

func main() {
	fmt.Println("=== 实验四：锁的进阶技巧与粒度优化 / Channel 信号量 ===")
	fmt.Println("目标: 由演示者决定何时放入下一波任务，再观察 Channel 如何把并发数卡在 3 个以内。")
	printHelp()

	demo := newDemoState(10, 3, 900*time.Millisecond)
	reader := bufio.NewReader(os.Stdin)

	for {
		if demo.completed.Load() == int64(demo.total) {
			fmt.Println("[结束] 全部 Worker 已完成。")
			return
		}

		fmt.Print("channel_semaphore> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Println("[结束] 输入流结束，演示退出。")
				return
			}
			fmt.Printf("[错误] 读取命令失败: %v\n", err)
			return
		}

		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "status":
			demo.printStatus()
		case "wave":
			if len(fields) != 2 {
				fmt.Println("[提示] 用法: wave 6")
				continue
			}
			count, err := strconv.Atoi(fields[1])
			if err != nil || count <= 0 {
				fmt.Println("[提示] wave 后面必须跟正整数")
				continue
			}
			demo.dispatch(count)
		case "wait":
			fmt.Println("[操作] 等待当前已派发 Worker 全部结束...")
			demo.waitDispatched()
			fmt.Println("[操作] 当前波次已全部结束。")
		case "quit":
			fmt.Println("[结束] 演示被手动终止。")
			return
		default:
			printHelp()
		}
	}
}
