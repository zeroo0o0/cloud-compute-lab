# 实验七：Raft 领导者选举演示说明

本目录对应第 5 章实验七，演示 Raft 领导者选举与故障转移的基本流程。

---

## 入口与代码分层

- 启动入口：`exp7/main.go`
  - 解析参数：`seed`、`kill-after-ms`
  - 创建节点、启动 goroutine、等待 Leader 当选

> 说明：本实验主要用单文件展示完整流程，便于课堂讲解与阅读。

---

## 启动方式

在 `CourseCode/ch5` 目录执行：

```powershell
go run ./exp7 -seed 7 -kill-after-ms 450
```

参数说明：

- `-seed`：随机种子（影响 election timeout 的随机分布）
- `-kill-after-ms`：首任 Leader 当选后多久触发宕机模拟（毫秒）

---

## 实验要点

- Follower 随机超时触发选举
- Candidate 拉票、获得多数派后当选 Leader
- Leader 定期心跳维持领导权
- Leader 故障后，其余节点自动选举新 Leader
