

# Cloud Compute Book / Warzone 多人对战游戏

本项目是云计算课程的配套教材项目，包含一个支持多人在同一局域网内对战的游戏系统以及相关实验。

## 项目简介

这是一个支持多人在同一局域网内对战的小型游戏项目。玩家可以在地图上移动、拾取武器、攻击对手，目标是击败其他玩家。游戏包含完整的账号与战绩系统，支持玩家数据持久化存储。

项目包含两个完整的游戏实现版本以及对应的实验课程：

- **C++ 版本**：v3.0 单体原型，支持多人联机、账号系统、战绩系统
- **Go 版本**：TUI 终端界面演示，支持多人对战、心跳检测、自动断连
- **Lab1**：网络编程实验 - 双人对战游戏（C/S 架构）
- **Lab2**：并发编程实验 - 多人开放世界对战（Goroutine + 锁）

## 项目结构

```
cloud-compute-book-code/
├── Lab/
│   ├── Lab1/                      # 实验一：网络编程
│   │   ├── README.md             # 实验说明
│   │   ├── student/              # 学生实现框架
│   │   │   ├── cmd/
│   │   │   │   ├── client/       # 客户端程序
│   │   │   │   └── server/       # 服务器程序
│   │   │   ├── protocol/         # 网络协议定义
│   │   │   │   └── message.go    # 消息结构与编解码
│   │   │   └── game/             # 游戏逻辑
│   │   │       └── game.go       # 双人对战逻辑
│   │   └── test/                 # 测试代码
│   │       ├── autotest.go      # 自动测试
│   │       └── run_test.sh      # 测试脚本
│   │
│   └── Lab2/                      # 实验二：并发编程
│       ├── README.md             # 实验说明
│       ├── student/              # 学生实现框架
│       │   ├── cmd/
│       │   │   ├── client/       # 客户端程序
│       │   │   └── server/       # 服务器程序
│       │   ├── protocol/         # 网络协议定义
│       │   │   └── message.go    # 消息结构与编解码
│       │   └── world/            # 世界状态管理
│       │       └── world.go      # 并发安全的游戏世界
│       └── test/                 # 测试代码
│           ├── autotest.go      # 自动测试
│           └── run_test.sh      # 测试脚本
│
├── warzone/
│   ├── c++/                      # C++ 实现 (v3.0 单体原型)
│   │   ├── client.cpp           # 客户端程序
│   │   ├── server.cpp           # 服务器程序
│   │   ├── database.h           # 数据库操作与密码哈希
│   │   ├── protocol.h           # 网络协议与游戏参数定义
│   │   ├── Makefile             # 编译配置
│   │   └── README.md           # C++ 版本说明
│   │
│   └── go/                       # Go 语言实现 (TUI 多人对战演示)
│       ├── cmd/
│       │   ├── client/          # 客户端程序
│       │   └── server/          # 服务器程序
│       ├── internal/
│       │   ├── client/          # 客户端核心逻辑
│       │   │   ├── backend.go
│       │   │   ├── net_backend.go
│       │   │   └── tui/         # 终端 UI 实现
│       │   ├── proto/          # 协议定义
│       │   └── server/         # 服务器核心逻辑与存储
│       ├── game.db             # SQLite 数据库文件
│       ├── go.mod
│       └── README.md           # Go 版本说明
│
└── README.md                    # 主 README 文件
```

## 主要功能

### 通用功能

- **多人联机**：支持多玩家在同一局域网内对战
- **账号系统**：玩家注册与登录，数据持久化存储
- **战绩系统**：记录玩家胜负、击杀、死亡等数据
- **实时状态同步**：服务器广播游戏状态给所有客户端

### 游戏玩法

- **移动控制**：玩家通过方向键在地图上移动
- **武器系统**：地图上随机生成武器，拾取后获得攻击能力
- **对战机制**：玩家可对视野内的对手发起攻击
- **生命值系统**：每位玩家有生命值，被攻击后扣血

## 实验课程

### Lab1：网络编程 - 双人对战游戏

学习使用 Go 语言实现基本的 C/S 架构网络通信。

**实验任务：**
- 任务 A：实现网络消息收发（`protocol/message.go`）
  - A-1: `Send(msg Message) error`
  - A-2: `Receive() (Message, error)`
- 任务 B：实现游戏逻辑（`game/game.go`）
  - B-1: `handleMove(p *Player, dir string) string`
  - B-2: `handleAttack(actor, target *Player) string`

**运行方法：**
```bash
cd Lab/Lab1/student
# 终端1：启动服务器
go run ./cmd/server
# 终端2：启动客户端1
go run ./cmd/client
# 终端3：启动客户端2
go run ./cmd/client
```

**测试方法：**
```bash
cd Lab/Lab1/test
./run_test.sh
```

### Lab2：并发编程 - 多人开放世界对战

学习使用 Goroutine 和互斥锁实现并发安全的游戏世界。

**实验任务：**
- 任务 C：并发安全的世界状态（`world/world.go`）
  - C-1: `AddPlayer`（写锁）
  - C-2: `RemovePlayer`（写锁）
  - C-3: `MovePlayer`（写锁）
  - C-4: `AttackPlayer`（写锁 + 死亡复活 Goroutine）
  - C-5: `GetSnapshot`（读锁）
- 任务 D：Goroutine 启动（`cmd/server/main.go`）
  - D-1: 启动广播 Goroutine
  - D-2: 为每个连接启动独立 Goroutine
- 任务 E：客户端接收 Goroutine（`cmd/client/main.go`）

**运行方法：**
```bash
cd Lab/Lab2/student
# 终端1：启动服务器（支持任意多玩家）
go run ./cmd/server
# 终端2、3、4...：多个客户端同时连接
go run ./cmd/client
```

**数据竞争检测：**
```bash
go run -race ./cmd/server
go run -race ./cmd/client
```

## C++ 版本 (v3.0)

### 编译与运行

```bash
cd warzone/c++
make
```

### 启动服务器

```bash
./server [端口号]
# 默认端口 8888
```

### 启动客户端

```bash
./client [服务器IP] [端口号]
# 默认连接 localhost:8888
```

### 操作说明

- **方向键**：移动
- **空格键**：攻击
- **Q**：查看战绩
- **ESC**：退出游戏

### 可调参数

在 `protocol.h` 中可修改以下参数：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `MAX_PLAYERS` | 最大玩家数 | 4 |
| `MAP_W / MAP_H` | 地图尺寸 | 20x20 |
| `MAX_HEALTH` | 最大生命值 | 100 |
| `ATTACK_DAMAGE` | 普通攻击伤害 | 10 |
| `POWER_DAMAGE` | 强力攻击伤害 | 25 |
| `ATTACK_RANGE` | 攻击范围 | 3 |
| `HEARTBEAT_INTERVAL` | 心跳间隔 | 3秒 |
| `HEARTBEAT_TIMEOUT` | 心跳超时时间 | 10秒 |

## Go 版本 (TUI)

### 编译与运行

```bash
cd warzone/go
go build ./cmd/server
go build ./cmd/client
```

### 启动服务器

```bash
./server
# 默认监听 :9000
```

### 启动客户端

```bash
./client
# 启动后输入服务器地址和用户名
```

### 操作说明

- **方向键**：移动
- **Tab**：选择挑战对手
- **Enter**：确认挑战
- **Esc**：退出

## 技术特点

### C++ 版本

- 使用原始套接字实现网络通信
- SQLite 作为数据存储
- 心跳机制检测玩家在线状态
- 互斥锁保护共享游戏状态

### Go 版本

- 使用 TUI 库实现终端界面
- JSON 协议通信
- SQLite 存储玩家数据
- 心跳监控与自动断连

### 实验课程特点

- **Lab1**：专注于网络编程基础，理解 TCP 通信协议设计
- **Lab2**：学习并发编程，掌握 Goroutine 和互斥锁的使用，了解数据竞争检测

## 许可证

本项目仅供学习与研究使用。