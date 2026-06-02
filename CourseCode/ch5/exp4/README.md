# 实验四：三种迁移模式对比

本实验用两个终端模拟玩家状态从 Server-A 迁移到 Server-B，并对比三种迁移策略：

- Stop-and-Copy：冻结玩家后，一次性迁移完整状态。
- Pre-Copy：玩家在线时先后台预复制，停机时只同步最后脏页。
- Wave-Based：按优先级分波迁移，停机时只传关键状态，恢复后继续后台补齐。

每种模式还带有一个独立 Client，用第三个终端模拟玩家持续发起操作。Client 会每 100ms 发送一次玩家请求，迁移前看到 Server-A 正常响应，停机期看到 `waiting...`，迁移完成后收到重定向并切换到 Server-B，从而直接观察“玩家感知卡顿时间”。

为了避免客户端长日志在窄窗口里自动换行，建议 Client 终端至少保持 120 列宽；窗口太窄时，输出会折行，影响刷新显示效果。

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
│   │   └── main.go                     # Stop-and-Copy 源端：停机后发送完整 1000MB 状态
│   ├── serverB/
│   │   └── main.go                     # Stop-and-Copy 目标端：接收完整状态并回 DONE
│   └── client/
│       └── main.go                     # Stop-and-Copy 客户端：持续发请求并观察停机卡顿
├── pre_copy/
│   ├── serverA/
│   │   └── main.go                     # Pre-Copy 源端：在线期传全量和多轮脏页，停机期传最后 80MB
│   ├── serverB/
│   │   └── main.go                     # Pre-Copy 目标端：按阶段接收 round0/dirty/final 数据块
│   └── client/
│       └── main.go                     # Pre-Copy 客户端：观察最后脏页同步造成的短暂停机
└── wave_based/
    ├── serverA/
    │   └── main.go                     # Wave-Based 源端：在线预热，停机传 10MB 关键状态，恢复后后台补齐
    ├── serverB/
    │   └── main.go                     # Wave-Based 目标端：按优先级阶段接收数据块并确认
    └── client/
        └── main.go                     # Wave-Based 客户端：观察关键状态到达后的快速恢复
```

## 实验流程

每种场景都包含三个程序：

- Server-A：模拟源服务器，发起迁移。
- Server-B：模拟目标服务器，接收状态并返回确认。
- Client：模拟玩家客户端，持续发送操作并在迁移完成后切换到 Server-B。

课堂演示时，每个场景开三个终端。先启动 Server-B，再启动 Server-A，再启动 Client。观察 Client 正常收到 `OK A` 后，在 Server-A 中按 `Enter` 开始迁移，迁移完成后 Server-A 自动退出。

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

终端 3 启动客户端：

```powershell
go run ./stop_and_copy/client
```

看到 Client 连续输出 `OK A` 后，在 Server-A 中按 `Enter` 开始迁移。

### Pre-Copy

终端 1 启动目标端：

```powershell
go run ./pre_copy/serverB
```

终端 2 启动源端：

```powershell
go run ./pre_copy/serverA
```

终端 3 启动客户端：

```powershell
go run ./pre_copy/client
```

看到 Client 连续输出 `OK A` 后，在 Server-A 中按 `Enter` 开始迁移。

### Wave-Based

终端 1 启动目标端：

```powershell
go run ./wave_based/serverB
```

终端 2 启动源端：

```powershell
go run ./wave_based/serverA
```

终端 3 启动客户端：

```powershell
go run ./wave_based/client
```

看到 Client 连续输出 `OK A` 后，在 Server-A 中按 `Enter` 开始迁移。

## 默认端口

- Stop-and-Copy：迁移端口 `127.0.0.1:9101`，Server-A 客户端端口 `127.0.0.1:9201`，Server-B 客户端端口 `127.0.0.1:9301`
- Pre-Copy：迁移端口 `127.0.0.1:9102`，Server-A 客户端端口 `127.0.0.1:9202`，Server-B 客户端端口 `127.0.0.1:9302`
- Wave-Based：迁移端口 `127.0.0.1:9103`，Server-A 客户端端口 `127.0.0.1:9203`，Server-B 客户端端口 `127.0.0.1:9303`

可选环境变量：

- Server-B：`LISTEN_ADDR` 设置迁移监听地址，`CLIENT_ADDR` 设置 Server-B 面向 Client 的监听地址
- Server-A：`TARGET_ADDR` 设置迁移目标地址，`CLIENT_ADDR` 设置 Server-A 面向 Client 的监听地址，`SERVER_B_CLIENT_ADDR` 设置迁移后发给 Client 的 Server-B 地址

## 客户端观察

Client 的关键输出如下：

```text
[Client][Pre-Copy] action #12 -> OK A 12, latency=0.32ms
[Client][Pre-Copy] action #13 -> waiting...
[Client][Pre-Copy] action #13 -> 迁移完成，切换到 Server-B=127.0.0.1:9302，本次请求等待=8.71ms，Server-A停机=8.54ms
[Client][Pre-Copy] action #14 -> OK B 14, latency=0.28ms
```

其中 `waiting...` 表示玩家请求已经进入停机窗口；“本次请求等待”是 Client 从发出该次操作到收到 Server-A 重定向的端到端等待时间；“Server-A停机”是 Server-A 从冻结玩家到收到 Server-B 确认的迁移停机时间。Wave-Based 的停机窗口很短，Client 请求可能没有刚好撞上冻结窗口，此时“本次请求等待”可能接近 0，但“Server-A停机”仍能稳定展示真实迁移停机时间。

## 三种方法对比

### Stop-and-Copy

流程：

```text
玩家正常在线
冻结玩家输入
停机期传完整 1000MB 状态
Server-B 接收完成并恢复玩家
```

观察重点：

- 总传输量：`1000MB`
- 停机期传输量：`1000MB`
- 玩家感知停机时间最大

### Pre-Copy

流程：

```text
玩家继续在线
后台先传 1000MB 全量状态
后台继续传 320MB、160MB 脏页
冻结玩家输入
停机期只传最后 80MB 脏页
Server-B 接收完成并恢复玩家
```

观察重点：

- 总传输量：`1560MB`
- 停机期传输量：`80MB`
- 玩家感知停机时间明显小于 Stop-and-Copy
- 代价是总传输量超过完整状态大小，因为脏页会被重复同步

### Wave-Based

流程：

```text
玩家继续在线
后台先传 500MB 非关键状态
冻结玩家输入
停机期只传 10MB 关键状态
Server-B 收到关键状态后立即恢复玩家
玩家恢复后，后台继续补齐剩余状态
```

观察重点：

- 总传输量：约 `1000MB`
- 停机期传输量：`10MB`
- 玩家感知停机时间最小
- 适合说明“先恢复玩家关键路径，再慢慢补齐非关键状态”的思想