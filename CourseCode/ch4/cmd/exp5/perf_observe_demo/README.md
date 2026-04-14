# 性能观测工具实验指导

## 一、实验定位

- **实验主题：** 性能观测工具演示
- **适用章节：** 第四章实验五“高并发性能榨取”
- **核心目标：** 使用 Go 自带的 benchmark 与 pprof 工具，分别定位 CPU 热点、内存分配、锁竞争与 goroutine 泄漏问题，并通过错误版 / 修复版对照理解常见优化思路。

本目录中的示例均为教学用最小样例，不追求业务复杂度，重点在于完整呈现“发现问题、采集数据、定位瓶颈、修改代码、再次验证”的基本流程。

## 二、目录说明

- `workload.go`：包含 CPU、内存分配、锁竞争三类问题的错误版与修复版代码。
- `benchmark_test.go`：包含三组 benchmark，用于生成 CPU、Heap、Mutex 观测数据。
- `main.go`：提供 goroutine 泄漏与修复版 HTTP 观测入口。

对应关系如下：

| 问题类型 | 反例函数 | 修复函数 | 观测入口 |
| --- | --- | --- | --- |
| CPU 热点 | `buildRankDigestSlow` | `buildRankDigestFast` | `BenchmarkCPUHotspotBad/Good` |
| 内存分配 | `encodeBattleLogBad` | `encodeBattleLogGood` | `BenchmarkHeapAllocBad/Good` |
| 锁竞争 | `mergeRoomDamageBad` | `mergeRoomDamageGood` | `BenchmarkMutexContentionBad/Good` |
| goroutine 泄漏 | `-mode leak` | `-mode fixed` | `go run ./cmd/exp5/perf_observe_demo` |

## 三、环境准备

### 1. Go 工具链检查

进入章节目录：

```powershell
cd E:\work\cloud-compute-book-code\CourseCode\ch4
```

检查 Go 与 pprof 是否可用：

```powershell
go version
go tool pprof -h
```

### 2. Graphviz 安装

若需要使用 `go tool pprof -http=:8081` 打开火焰图网页，需预先安装 Graphviz。

Windows：

```powershell
winget install --id Graphviz.Graphviz -e
```

若 `winget` 由于代理、证书或源同步问题不可用，可改用 Graphviz 官方安装包或官方 zip 包。

macOS：

```bash
brew install graphviz
```

Ubuntu / Debian：

```bash
sudo apt-get update
sudo apt-get install -y graphviz
```

安装完成后执行：

```powershell
dot -V
```

## 四、工具与指标对照

| 瓶颈类型 | 推荐工具 | 核心命令 | 重点指标 |
| --- | --- | --- | --- |
| CPU 热点 | `pprof CPU` | `go test -cpuprofile` | Top 函数、火焰图 |
| 内存分配 | `testing.B + benchmem + pprof heap` | `go test -benchmem -memprofile` | `B/op`、`allocs/op` |
| 锁竞争 | `pprof mutex` | `go test -mutexprofile` | 阻塞时间集中位置 |
| goroutine 泄漏 | `net/http/pprof` | `go run` + `/debug/pprof/goroutine` | goroutine 数量趋势、阻塞栈 |

## 五、实验内容与操作步骤

### （一）CPU 热点观测

- **问题场景：** 排行榜摘要计算中，将排序逻辑放入高频循环，导致重复计算。
- **目标函数：** `buildRankDigestSlow` / `buildRankDigestFast`
- **核心代码位置：** `workload.go:15-40`

操作步骤：

1. 运行错误版 benchmark 并采集 CPU profile。

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkCPUHotspotBad -benchtime 2s -cpuprofile cpu_bad.prof
go tool pprof -top cpu_bad.prof
```

2. 如需展示火焰图，可执行：

```powershell
go tool pprof -http=:8081 cpu_bad.prof
```

3. 重点观察 `sort.Strings`、排序相关调用以及 `buildRankDigestSlow` 是否位于热点区域。
4. 运行修复版并再次对比结果。

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkCPUHotspotGood -benchtime 2s -cpuprofile cpu_good.prof
go tool pprof -top cpu_good.prof
go tool pprof -http=:8081 cpu_good.prof
```

- **预期现象：** 修复版应将排序移出热路径，CPU 时间占比明显下降。
- **阅读代码时建议重点查看：**
  - `buildRankDigestSlow`：`workload.go:15-30`
  - `buildRankDigestFast`：`workload.go:32-40`

### （二）内存分配观测

- **问题场景：** 日志编码过程中，每次处理事件都创建新的 `bytes.Buffer`，造成较多短命对象。
- **目标函数：** `encodeBattleLogBad` / `encodeBattleLogGood`
- **核心代码位置：** `workload.go:43-67`

操作步骤：

1. 运行错误版 benchmark，并同时输出 `benchmem` 与 heap profile。

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkHeapAllocBad -benchmem -memprofile heap_bad.prof
go tool pprof -top -alloc_space heap_bad.prof
go tool pprof -http=:8081 -alloc_space heap_bad.prof
```

2. 先观察 benchmark 输出中的 `B/op` 与 `allocs/op`。
3. 再观察 heap profile 中是否集中出现 `bytes.Buffer`、`fmt.Fprintf` 与 `encodeBattleLogBad`。
4. 运行修复版并对比分配数据。

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkHeapAllocGood -benchmem -memprofile heap_good.prof
go tool pprof -top -alloc_space heap_good.prof
go tool pprof -http=:8081 -alloc_space heap_good.prof
```

- **预期现象：** 修复版复用缓冲区后，`allocs/op` 与 `B/op` 应明显下降。
- **阅读代码时建议重点查看：**
  - `encodeBattleLogBad`：`workload.go:43-57`
  - `encodeBattleLogGood`：`workload.go:59-67`

### （三）锁竞争观测

- **问题场景：** 多个 worker 并发更新房间总伤害时，每次更新都进入同一临界区。
- **目标函数：** `mergeRoomDamageBad` / `mergeRoomDamageGood`
- **核心代码位置：** `workload.go:70-118`

操作步骤：

1. 运行错误版 benchmark 并采集 mutex profile。

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkMutexContentionBad -benchtime 2s -mutexprofile mutex_bad.prof -mutexprofilefraction=1
go tool pprof -top mutex_bad.prof
go tool pprof -http=:8081 mutex_bad.prof
```

2. 重点观察 `sync.(*Mutex).Lock` 与 `mergeRoomDamageBad` 是否集中占用阻塞时间。
3. 运行修复版并对比。

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkMutexContentionGood -benchtime 2s -mutexprofile mutex_good.prof -mutexprofilefraction=1
go tool pprof -top mutex_good.prof
go tool pprof -http=:8081 mutex_good.prof
```

- **预期现象：** 修复版采用“本地累计、一次合并”的方式后，锁竞争时间应明显减少。
- **阅读代码时建议重点查看：**
  - `mergeRoomDamageBad`：`workload.go:70-95`
  - `mergeRoomDamageGood`：`workload.go:97-118`

### （四）goroutine 泄漏观测

- **问题场景：** 程序持续创建 goroutine，但未提供有效退出路径，导致协程数量不断增长。
- **运行模式：** `-mode leak` / `-mode fixed`
- **核心代码位置：** `main.go:13-57`，入口位于 `main.go:59-98`

操作步骤：

1. 启动错误版：

```powershell
go run ./cmd/exp5/perf_observe_demo -mode leak -seconds 20
```

2. 在另一终端中采集 goroutine profile：

```powershell
go tool pprof -top http://127.0.0.1:6060/debug/pprof/goroutine
go tool pprof -http=:8081 http://127.0.0.1:6060/debug/pprof/goroutine
```

或直接查看文本栈：

```powershell
(Invoke-WebRequest http://127.0.0.1:6060/debug/pprof/goroutine?debug=1).Content
```

3. 观察 goroutine 总数是否持续上升，以及阻塞栈是否反复落在同一段等待代码上。
4. 运行修复版并再次对比：

```powershell
go run ./cmd/exp5/perf_observe_demo -mode fixed -seconds 20
```

- **预期现象：** 错误版的 goroutine 数量会持续增长；修复版的 goroutine 能够正常结束，数量保持稳定。
- **阅读代码时建议重点查看：**
  - `startLeakDemo`：`main.go:13-34`
  - `startFixedDemo`：`main.go:36-57`
  - `main`：`main.go:59-98`

## 六、建议演示顺序

若课堂时间有限，可按以下顺序进行：

```powershell
go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkCPUHotspotBad -benchtime 2s -cpuprofile cpu_bad.prof
go tool pprof -top cpu_bad.prof

go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkHeapAllocBad -benchmem

go test ./cmd/exp5/perf_observe_demo -run '^$' -bench BenchmarkMutexContentionBad -benchtime 2s -mutexprofile mutex_bad.prof -mutexprofilefraction=1
go tool pprof -top mutex_bad.prof

go run ./cmd/exp5/perf_observe_demo -mode leak -seconds 20
```

该顺序能够覆盖 PPT 中的四类典型瓶颈，并能在有限时间内完成“现象、工具、指标、修复思路”的完整展示。

## 七、成功标准

- 能够正确运行 benchmark 与 pprof 相关命令。
- 能够指出 CPU profile 中的热点函数位置。
- 能够根据 `benchmem` 输出识别高分配来源。
- 能够根据 mutex profile 说明阻塞主要集中在哪一段临界区代码。
- 能够根据 goroutine profile 或文本栈判断是否存在协程泄漏。
- 能够说明错误版与修复版在性能观测结果上的差异。

## 八、实验结束后的清理

```powershell
Remove-Item cpu_bad.prof,cpu_good.prof,heap_bad.prof,heap_good.prof,mutex_bad.prof,mutex_good.prof -ErrorAction SilentlyContinue
```
