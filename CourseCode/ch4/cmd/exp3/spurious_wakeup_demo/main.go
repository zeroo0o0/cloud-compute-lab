package main

import (
	"fmt"
	"sync"
)

type sharedQueue struct {
	mu    sync.Mutex
	cond  *sync.Cond
	queue []int
}

func newSharedQueue() *sharedQueue {
	q := &sharedQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func runWrongVersion() {
	q := newSharedQueue()
	waiting := make(chan struct{})
	done := make(chan struct{})

	go func() {
		q.mu.Lock()

		/*
			================ 【学生重点 实验三：错误版 if wait】 ================
			这段代码就对应 PPT 里那种错误写法：

			    if len(queue) == 0 {
			        cond.Wait()
			    }
			    item = queue[0]

			问题是：Wait 返回以后，代码直接往下取数据，
			没有重新检查队列是不是仍然为空。
			==============================================================
		*/
		if len(q.queue) == 0 {
			fmt.Println("[错误版] queue empty -> Wait")
			close(waiting)
			q.cond.Wait()
			fmt.Println("[错误版] 被唤醒后直接继续执行")
		}

		if len(q.queue) == 0 {
			fmt.Println("[错误结果] queue 还是空的，但 if 版本已经走到 dequeue 这一步。")
			q.mu.Unlock()
			close(done)
			return
		}

		item := q.queue[0]
		q.queue = q.queue[1:]
		q.mu.Unlock()
		fmt.Printf("[错误版] 取出元素: %d\n", item)
		close(done)
	}()

	<-waiting

	q.mu.Lock()
	fmt.Println("[系统] 只发 Signal，不入队任何元素。")
	q.cond.Signal()
	q.mu.Unlock()

	<-done
}

func runRightVersion() {
	q := newSharedQueue()
	waiting := make(chan struct{})
	rewaiting := make(chan struct{})
	done := make(chan struct{})

	go func() {
		q.mu.Lock()
		firstWait := true

		/*
			================ 【学生重点 实验三：正确版 for wait】 ================
			正确写法必须是：

			    for len(queue) == 0 {
			        cond.Wait()
			    }
			    item = queue[0]

			Wait 返回只表示“你可以再检查一次了”，
			不表示“队列里现在一定已经有数据了”。
			===============================================================
		*/
		for len(q.queue) == 0 {
			if firstWait {
				fmt.Println("[正确版] queue empty -> Wait")
				close(waiting)
				firstWait = false
			} else {
				fmt.Println("[正确版] 再次检查发现 queue 还是空的 -> 继续 Wait")
				close(rewaiting)
			}
			q.cond.Wait()
		}

		item := q.queue[0]
		q.queue = q.queue[1:]
		q.mu.Unlock()
		fmt.Printf("[正确结果] queue 非空，安全取出元素: %d\n", item)
		close(done)
	}()

	<-waiting

	q.mu.Lock()
	fmt.Println("[系统] 先发一次假 Signal，不入队元素。")
	q.cond.Signal()
	q.mu.Unlock()

	<-rewaiting

	q.mu.Lock()
	fmt.Println("[系统] 这次真正入队 42，然后再 Signal。")
	q.queue = append(q.queue, 42)
	q.cond.Signal()
	q.mu.Unlock()

	<-done
}

func main() {
	fmt.Println("=== 虚假唤醒极简演示 ===")
	fmt.Println("目标：只展示 queue empty + cond.Wait 的正确/错误写法。")
	fmt.Println()

	runWrongVersion()

	fmt.Println()
	fmt.Println("---- 换成正确写法 ----")

	runRightVersion()

	fmt.Println()
	fmt.Println("[结论] RemoveFromQueue 这种代码里，Wait 外面必须套 for，不能只写 if。")
}
