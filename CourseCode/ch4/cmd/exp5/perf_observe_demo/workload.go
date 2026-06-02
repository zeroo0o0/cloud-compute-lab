package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"sync"
)

var heroNames = []string{
	"archer", "knight", "mage", "healer", "rogue", "tank", "hunter", "bard",
}

var _ = strconv.Itoa // 引入 strconv 包，避免编译器优化掉 strconv.Itoa 函数

func buildRankDigestSlow(rounds int) [32]byte {
	var digest [32]byte

	for round := 0; round < rounds; round++ {
		/*
			================ 【学生重点 实验五：CPU 热点反例】 ================
			这里每一轮都复制并排序一份名字列表。
			排序本身不复杂，但被放进高频循环后，会成为 CPU profile 里的热点。
			=================================================================
		*/
		names := append([]string(nil), heroNames...)
		sort.Strings(names)
		digest = sha256.Sum256([]byte(fmt.Sprintf("%s:%d", names[round%len(names)], round)))
	}
	return digest
}

func buildRankDigestFast(rounds int) [32]byte {
	names := append([]string(nil), heroNames...)
	sort.Strings(names)

	var digest [32]byte
	for round := 0; round < rounds; round++ {
		// digest = sha256.Sum256([]byte(fmt.Sprintf("%s:%d", names[round%len(names)], round)))
		digest = sha256.Sum256([]byte(names[round%len(names)] + ":" + strconv.Itoa(round)))
	}
	return digest
}

func encodeBattleLogBad(events int) int {
	total := 0
	for i := 0; i < events; i++ {
		/*
			================ 【学生重点 实验五：Heap 反例】 ================
			这里每条日志都 new 一个临时 Buffer。
			benchmem 和 heap profile 会把分配来源指到这类短命对象上。
			===============================================================
		*/
		buf := new(bytes.Buffer)
		fmt.Fprintf(buf, "event=%d hero=%d x=%d y=%d", i, i%16, i%1024, (i*7)%1024)
		total += buf.Len()
	}
	return total
}

func encodeBattleLogGood(events int) int {
	var buf bytes.Buffer
	total := 0
	for i := 0; i < events; i++ {
		buf.Reset()
		fmt.Fprintf(&buf, "event=%d hero=%d x=%d y=%d", i, i%16, i%1024, (i*7)%1024)
		total += buf.Len()
	}
	return total
}

func mergeRoomDamageBad(workers, itemsPerWorker int) int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	total := 0

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for item := 0; item < itemsPerWorker; item++ {
				/*
					================ 【学生重点 实验五：Mutex 反例】 ================
					这里每次累计伤害都抢同一把锁。
					运行 mutex profile 时，阻塞时间会集中到这段 Lock/Unlock 附近。
					===============================================================
				*/
				mu.Lock()
				total += (worker + item) & 3
				mu.Unlock()
			}
		}(worker)
	}

	wg.Wait()
	return total
}

func mergeRoomDamageGood(workers, itemsPerWorker int) int {
	var mu sync.Mutex
	var wg sync.WaitGroup
	total := 0

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			local := 0
			for item := 0; item < itemsPerWorker; item++ {
				local += (worker + item) & 3
			}

			mu.Lock()
			total += local
			mu.Unlock()
		}(worker)
	}

	wg.Wait()
	return total
}
