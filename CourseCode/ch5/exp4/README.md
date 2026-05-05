# 实验四：三种迁移模式对比

本实验用两个终端模拟玩家状态从 Server-A 迁移到 Server-B，并对比三种迁移策略：

- Stop-and-Copy：冻结玩家后，一次性迁移完整状态。
- Pre-Copy：玩家在线时先后台预复制，停机时只同步最后脏页。
- Wave-Based：按优先级分波迁移，停机时只传关键状态，恢复后继续后台补齐。

为了让课堂演示聚焦迁移策略差异，三种模式都使用单 TCP 通道传输真实字节数据，并去掉序列化/反序列化等额外成本。重点观察：

- 总传输量
- 停机期传输量
- 玩家感知停机时间

## 目录结构与文件作用

```text
exp4/
├── go.mod                              # 独立模块：exp4
├── README.md                           # 本文件
├── internal/
│   └── migproto/
│       └── protocol.go                 # 共享大小常量、模拟数据构造、字节大小格式化工具
├── stop_and_copy/
│   ├── serverA/
│   │   └── main.go                     # Stop-and-Copy 源端：停机后发送完整 50MB 状态
│   └── serverB/
│       └── main.go                     # Stop-and-Copy 目标端：接收完整状态并回 DONE
├── pre_copy/
│   ├── serverA/
│   │   └── main.go                     # Pre-Copy 源端：在线期传全量和多轮脏页，停机期传最后 1MB
│   └── serverB/
│       └── main.go                     # Pre-Copy 目标端：按阶段接收 round0/dirty/final 数据块
└── wave_based/
    ├── serverA/
    │   └── main.go                     # Wave-Based 源端：在线预热，停机传 256KB 关键状态，恢复后后台补齐
    └── serverB/
        └── main.go                     # Wave-Based 目标端：按优先级阶段接收数据块并确认
```

## 实验流程

每种场景都包含两个程序：

- Server-A：模拟源服务器，发起迁移。
- Server-B：模拟目标服务器，接收状态并返回确认。

课堂演示时，每个场景开两个终端即可。先启动 Server-B，再启动 Server-A。Server-A 启动后按 `Enter` 开始迁移，迁移完成后 Server-A 自动退出。

三种模式的底层传输流程保持一致：

```text
Server-A 发送阶段头，说明接下来传什么、传多少字节
Server-A 发送真实字节数据
Server-B 接收完成后返回 DONE
Server-A 收到 DONE 后进入下一阶段或结束停机计时
```

这样可以把差异集中在“停机期到底传了多少数据”上。

## 运行方式

先进入实验目录：

```powershell
cd exp4
```

### Stop-and-Copy

终端 1 启动目标端：

```powershell
go run ./stop_and_copy/serverB
```

终端 2 启动源端：

```powershell
go run ./stop_and_copy/serverA
```

在 Server-A 中按 `Enter` 开始迁移。

### Pre-Copy

终端 1 启动目标端：

```powershell
go run ./pre_copy/serverB
```

终端 2 启动源端：

```powershell
go run ./pre_copy/serverA
```

在 Server-A 中按 `Enter` 开始迁移。

### Wave-Based

终端 1 启动目标端：

```powershell
go run ./wave_based/serverB
```

终端 2 启动源端：

```powershell
go run ./wave_based/serverA
```

在 Server-A 中按 `Enter` 开始迁移。

## 默认端口

- Stop-and-Copy：`127.0.0.1:9101`
- Pre-Copy：`127.0.0.1:9102`
- Wave-Based：`127.0.0.1:9103`

可选环境变量：

- Server-B：`LISTEN_ADDR`
- Server-A：`TARGET_ADDR`

## 三种方法对比

### Stop-and-Copy

流程：

```text
玩家正常在线
冻结玩家输入
停机期传完整 50MB 状态
Server-B 接收完成并恢复玩家
```

观察重点：

- 总传输量：`50MB`
- 停机期传输量：`50MB`
- 玩家感知停机时间最大

### Pre-Copy

流程：

```text
玩家继续在线
后台先传 50MB 全量状态
后台继续传 8MB、4MB、2MB 脏页
冻结玩家输入
停机期只传最后 1MB 脏页
Server-B 接收完成并恢复玩家
```

观察重点：

- 总传输量：`65MB`
- 停机期传输量：`1MB`
- 玩家感知停机时间明显小于 Stop-and-Copy
- 代价是总传输量超过完整状态大小，因为脏页会被重复同步

### Wave-Based

流程：

```text
玩家继续在线
后台先传 20MB 非关键状态
冻结玩家输入
停机期只传 256KB 关键状态
Server-B 收到关键状态后立即恢复玩家
玩家恢复后，后台继续补齐剩余状态
```

观察重点：

- 总传输量：约 `50MB`
- 停机期传输量：`256KB`
- 玩家感知停机时间最小
- 适合说明“先恢复玩家关键路径，再慢慢补齐非关键状态”的思想

## 课堂观察建议

主要看 Server-A 的最终输出：

```text
总传输量=...
停机期传输量=...
玩家感知停机时间=...
```

本实验在本机 `127.0.0.1` 上运行，耗时会受到 Go 调度、TCP 缓冲、系统负载影响，重点看趋势：

```text
Stop-and-Copy 停机期 50MB > Pre-Copy 停机期 1MB > Wave-Based 停机期 256KB
```
