# 性能观测工具实验指导

这个实验不是为了“跑一个 benchmark 就结束”，而是为了带学生完整走一遍：

1. 先制造一个性能问题。
2. 再用工具把问题抓出来。
3. 然后对照错误版 / 修复版，说明为什么会慢。

本目录一共覆盖 4 类常见问题：

- CPU 热点
- 内存分配过多
- 锁竞争
- goroutine 泄漏

---

## 一、目录里几个文件分别做什么

- `workload.go`
  放了三类“错误版 / 修复版”业务函数：CPU、内存分配、锁竞争。
- `benchmark_test.go`
  放了 benchmark 入口。`go test -bench ...` 跑的就是这里。
- `main.go`
  单独负责 goroutine 泄漏演示，因为它需要启动一个带 `pprof` 的 HTTP 服务，方便浏览器和 `go tool pprof` 在线抓取。

对应关系如下：

| 问题类型 | 错误版 | 修复版 | 入口 |
| --- | --- | --- | --- |
| CPU 热点 | `buildRankDigestSlow` | `buildRankDigestFast` | `BenchmarkCPUHotspotBad/Good` |
| 内存分配 | `encodeBattleLogBad` | `encodeBattleLogGood` | `BenchmarkHeapAllocBad/Good` |
| 锁竞争 | `mergeRoomDamageBad` | `mergeRoomDamageGood` | `BenchmarkMutexContentionBad/Good` |
| goroutine 泄漏 | `-mode leak` | `-mode fixed` | `go run ./cmd/exp5/perf_observe_demo` |

---

## 二、上课时建议怎么演示

如果课堂时间有限，建议按这个顺序：

1. 先跑一遍总 benchmark，让学生知道“确实有慢和快的差别”。
2. 再挑一个 CPU profile，讲怎么找热点函数。
3. 再挑一个 heap / mutex profile，讲怎么把“慢”翻译成“分配多”或“锁竞争重”。
4. 最后演示 goroutine 泄漏，让学生看到在线观测的效果。

最短命令顺序如下：

```powershell
cd .\CourseCode\ch4

go test ./cmd/exp5/perf_observe_demo -run '^$' -bench . -benchmem

go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkCPUHotspotBad -benchtime 2s -cpuprofile cpu_bad.prof
go tool pprof -top cpu_bad.prof

go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkMutexContentionBad -benchtime 2s -mutexprofile mutex_bad.prof -mutexprofilefraction=1
go tool pprof -top mutex_bad.prof

go run ./cmd/exp5/perf_observe_demo -mode leak -seconds 60
```

---

## 三、环境准备

### 1. 进入章节目录

```powershell
cd E:\work\cloud-compute-book-code\CourseCode\ch4
```

### 2. 检查 Go 和 pprof

```powershell
go version
go tool pprof -h
```

### 3. 检查 Graphviz

如果你要用网页端火焰图，最好先装 Graphviz。

Windows：

```powershell
winget install --id Graphviz.Graphviz -e
```

如果 `winget` 下载不了，也可以跳过网页端，只用 `go tool pprof -top` 一样可以完成课堂演示。

安装完成后检查：

```powershell
dot -V
```

---

## 四、先跑一遍总 benchmark

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench . -benchmem
```

你会看到类似下面几类信息：

- `ns/op`
  每次操作平均花多少纳秒。越小越快。
- `B/op`
  每次操作平均分配多少字节。越小越省内存。
- `allocs/op`
  每次操作平均发生多少次内存分配。越小越好。

课堂里可以先只讲一句：

- CPU 问题主要看 `ns/op`
- 内存问题主要看 `B/op` 和 `allocs/op`
- 后面再用 `pprof` 去看“到底慢在谁身上”

---

## 五、CPU 热点观测

### 1. 先抓错误版

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkCPUHotspotBad -benchtime 2s -cpuprofile cpu_bad.prof
go tool pprof -top cpu_bad.prof
```

这里要讲的核心是：

- `buildRankDigestSlow` 把排序放在高频路径里重复做了。
- 所以热点会集中到排序相关逻辑。

### 2. 如果要开网页端

```powershell
go tool pprof -http=:8081 cpu_bad.prof
```

注意这里推荐写：

```powershell
-http=:8081
```

不要写成：

```powershell
-http=127.0.0.1:8081
```

### 3. 再抓修复版

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkCPUHotspotGood -benchtime 2s -cpuprofile cpu_good.prof
go tool pprof -top cpu_good.prof

go tool pprof -http=:8082 cpu_good.prof
```

### 4. 课堂上重点怎么讲

- 错误版慢，不是因为“Go 慢”，而是因为重复排序。
- 修复版快，不是因为“用了黑科技”，而是因为把重复工作移出了热路径。

---

## 六、内存分配观测

### 1. 抓错误版

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkHeapAllocBad -benchmem -memprofile heap_bad.prof
go tool pprof -top -alloc_space heap_bad.prof
```

如果想开网页端：

```powershell
go tool pprof -http=:8082 -alloc_space heap_bad.prof
```

### 2. 先看 benchmark 输出

这里最该讲的是两列：

- `B/op`
  每次操作一共分配了多少内存。
- `allocs/op`
  每次操作分配了多少次。

如果错误版里每次都新建 `bytes.Buffer`，这两项通常都会比较高。

### 3. 再抓修复版

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkHeapAllocGood -benchmem -memprofile heap_good.prof
go tool pprof -top -alloc_space heap_good.prof
```

### 4. 课堂上重点怎么讲

- 这里不是“逻辑错了”，而是“分配太频繁了”。
- 频繁创建短命对象，会抬高 GC 压力。
- 修复版通过复用缓冲区，让 `allocs/op` 和 `B/op` 都下降。

---

## 七、锁竞争观测

### 1. 抓错误版

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkMutexContentionBad -benchtime 2s -mutexprofile mutex_bad.prof -mutexprofilefraction=1
go tool pprof -top mutex_bad.prof
```

如果想开网页端：

```powershell
go tool pprof -http=:8083 mutex_bad.prof
```

### 2. 这里该看什么

这类 profile 不是看“谁算得最久”，而是看：

- 谁因为锁被挡住了
- 阻塞时间主要堆在哪一段临界区

### 3. 再抓修复版

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkMutexContentionGood -benchtime 2s -mutexprofile mutex_good.prof -mutexprofilefraction=1
go tool pprof -top mutex_good.prof
```

### 4. 课堂上重点怎么讲

- 错误版的问题，不是“有锁就不行”，而是“锁拿得太碎、太频繁”。
- 修复版把很多次小更新变成“本地累计后统一合并”，所以锁竞争明显下降。

---

## 八、goroutine 泄漏观测

这一段最容易因为操作顺序错而失败，所以建议严格按下面两终端的顺序演示。

### 终端 1：先启动 leak 版

```powershell
go run ./cmd/exp5/perf_observe_demo -mode leak -seconds 60
```

你应该先看到这一行：

```text
[pprof] HTTP 服务已启动，正在监听 127.0.0.1:6060
```

只有看到这行以后，才去开第二个终端。

同时终端 1 会不断打印：

```text
[状态] goroutines=3
[状态] goroutines=23
[状态] goroutines=44
...
```

这表示 goroutine 数量在持续上涨。

### 终端 2：抓 goroutine profile

```powershell
go tool pprof -top http://127.0.0.1:6060/debug/pprof/goroutine
```

如果要开网页端：

```powershell
go tool pprof -http=:8084 http://127.0.0.1:6060/debug/pprof/goroutine
```

如果想直接看文字栈：

```powershell
(Invoke-WebRequest http://127.0.0.1:6060/debug/pprof/goroutine?debug=1).Content
```

### 这里为什么网页端不一定像 CPU 火焰图那样直观

因为 goroutine profile 的重点不是“谁最耗 CPU”，而是：

- 当前有哪些 goroutine
- 它们卡在什么地方
- 数量有没有一直涨

所以课堂上更推荐这样讲：

1. 先看终端 1 的 goroutine 数量趋势。
2. 再看 `pprof -top`，确认大量 goroutine 落在同一段等待逻辑。
3. 如果想给学生看更原始的证据，再用 `?debug=1` 看文本栈。

### 再跑 fixed 版

```powershell
go run ./cmd/exp5/perf_observe_demo -mode fixed -seconds 60
```

这时 goroutine 数量会保持稳定，不会一直往上长。

### 如果出现 `actively refused it`

通常是下面几种原因：

- 第一个终端里的 `go run` 还没真正监听成功。
- 第一个终端已经超时退出了。
- `6060` 端口被别的程序占用了。
- 你在第二个终端抓得太快了。

这时最稳的做法是换个端口重来：

```powershell
go run ./cmd/exp5/perf_observe_demo -mode leak -seconds 60 -addr 127.0.0.1:6061
go tool pprof -top http://127.0.0.1:6061/debug/pprof/goroutine
go tool pprof -http=:8085 http://127.0.0.1:6061/debug/pprof/goroutine
```

---

## 九、每个输出到底表示什么

### 1. `go test -bench ... -benchmem`

- `ns/op`
  平均一次操作花多久。
- `B/op`
  平均一次操作分配多少字节。
- `allocs/op`
  平均一次操作分配多少次对象。

### 2. `go tool pprof -top`

这是“按消耗排序的函数列表”。

课堂上只需要让学生先学会看两件事：

- 排名前几的函数是谁
- 这些函数是不是正好就是你怀疑的问题点

### 3. `go tool pprof -http=:端口`

这是网页端浏览器视图，可以看调用图、火焰图、Top。

它更适合“展示”，
`-top` 更适合“快速定位”。

### 4. `goroutine?debug=1`

这是最直白的文字证据：

- 当前一共有多少 goroutine
- 每批 goroutine 卡在哪个调用栈

---

## 十、课堂上一句话总结四类问题

- CPU 热点：同样的活做太多次了。
- 内存分配：短命对象建得太频繁了。
- 锁竞争：大家都挤在同一个临界区门口。
- goroutine 泄漏：任务结束了，但协程没退出。

---

## 十一、我建议你实际演示时这样收尾

每演示完一类问题，都让学生回答这三个问题：

1. 现象是什么？
2. 工具里哪个指标证明了这个现象？
3. 代码里哪种写法导致了这个现象？

这样学生记住的就不只是命令，而是“代码写法 -> 运行现象 -> 观测证据”这条链条。

---

## 十二、实验结束后清理 profile 文件

```powershell
Remove-Item cpu_bad.prof,cpu_good.prof,heap_bad.prof,heap_good.prof,mutex_bad.prof,mutex_good.prof -ErrorAction SilentlyContinue
```
